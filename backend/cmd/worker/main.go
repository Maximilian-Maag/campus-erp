// Command worker runs asynchronous jobs (role: worker): imports, exports, ETL,
// report generation. Queue backend is River on Postgres (ADR-0007).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
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
	client, err := jobs.New(pool, registry)
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
