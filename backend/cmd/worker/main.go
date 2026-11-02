// Command worker runs asynchronous jobs (role: worker): imports, exports, ETL,
// report generation. Queue backend is River on Postgres (ADR-0007).
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn, err := db.DSNFromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("migrations applied: %d", applied)

	// The ETL datasets read from the legacy database (docs/19). When no legacy DSN
	// is configured, the source is the canonical pool (dev/seed).
	var legacyPool *pgxpool.Pool
	if cfg.LegacyDSN != "" {
		lp, err := db.Open(ctx, cfg.LegacyDSN)
		if err != nil {
			log.Fatalf("legacy db: %v", err)
		}
		defer lp.Close()
		legacyPool = lp
	}
	srcPool := pool
	if legacyPool != nil {
		srcPool = legacyPool
	}

	store := audit.NewStore(pool)
	registry := etl.NewRegistry(
		etl.NewRunner(store).WithLedger(etl.NewAuditLedger(store)),
		lookup.StandardDatasets(srcPool, lookup.NewValueService(lookup.NewValueRepo(pool), store))...,
	)

	// River owns its own schema; apply it before the client starts.
	if err := jobs.Migrate(ctx, pool); err != nil {
		log.Fatalf("jobs: %v", err)
	}
	statementSvc := kasse.NewStatementService(kasse.NewStatementRepo(pool), audit.NewStore(pool))
	importer := func(ctx context.Context, accountID int64, content, by string) (int, error) {
		statements, err := statementSvc.Import(ctx, accountID, content, by)
		return len(statements), err
	}
	ledgerSvc := kasse.NewLedgerExportService(kasse.NewLedgerRepo(pool),
		audit.NewStore(pool), kasse.DefaultLedgerAccounts())
	exporter := func(ctx context.Context, from, to time.Time, by string) (int, error) {
		exp, err := ledgerSvc.Export(ctx, from, to, by)
		if err != nil {
			return 0, err
		}
		content, err := exp.Marshal()
		if err != nil {
			return 0, err
		}
		return len(exp.Postings), writeOutbox(content)
	}
	runRepo := kasse.NewPaymentRunRepo(pool)
	sepaExporter := func(ctx context.Context, runID int64, _ string) (string, string, error) {
		content, err := runRepo.RunFile(ctx, runID)
		if err != nil {
			return "", "", err
		}
		name := fmt.Sprintf("sepa-%d.xml", runID)
		if err := writeTo(strings.TrimSpace(os.Getenv("ERP_SEPA_OUTBOX_DIR")),
			"worker: ERP_SEPA_OUTBOX_DIR is not set", name, content); err != nil {
			return "", "", err
		}
		return name, content, nil
	}
	client, err := jobs.New(pool, registry, importer, exporter, sepaExporter)
	if err != nil {
		log.Fatalf("jobs: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		log.Fatalf("jobs: start: %v", err)
	}
	log.Printf("worker running (tenant=%q role=%q datasets=%d)", cfg.Tenant, cfg.Role, len(registry.Names()))

	<-ctx.Done()
	log.Printf("worker stopping")
	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.Stop(stopCtx); err != nil {
		log.Printf("jobs: stop: %v", err)
	}
}

// writeTo drops content into dir under name. It is the outbox seam: object storage or
// SFTP replaces this one function.
func writeTo(dir, missing, name, content string) error {
	if dir == "" {
		return errors.New(missing)
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}

// writeOutbox drops a ledger export into the configured outbox directory.
func writeOutbox(content string) error {
	name := fmt.Sprintf("ledger-postings-%s.csv", time.Now().UTC().Format("20060102T150405"))
	return writeTo(strings.TrimSpace(os.Getenv("ERP_LEDGER_OUTBOX_DIR")),
		"worker: ERP_LEDGER_OUTBOX_DIR is not set", name, content)
}
