// Command api is the HTTP entrypoint (role: api).
package main

import (
	"context"
	"fmt"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/documents"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/storage"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/reports"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()

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

	// buildServer wires the whole modular monolith over one tenant's pool. It is
	// the seam the multi-tenant front uses to build each tenant's server lazily.
	buildServer := func(pool *pgxpool.Pool) *httpapi.Server {
		store := audit.NewStore(pool)
		budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
		lookupSvc := lookup.NewValueService(lookup.NewValueRepo(pool), store)
		srcPool := pool
		if legacyPool != nil {
			srcPool = legacyPool
		}
		etlRunner := etl.NewRunner(store).WithLedger(etl.NewAuditLedger(store))
		etlRegistry := etl.NewRegistry(etlRunner, lookup.StandardDatasets(srcPool, lookupSvc)...)
		dunningRepo := kasse.NewDunningRepo(pool)
		dunningSvc := kasse.NewDunningService(dunningRepo, store)
		if issuer := dunningDocumentsFromEnv(); issuer != nil {
			dunningSvc = dunningSvc.WithDocuments(issuer)
		}
		receivableRepo := kasse.NewReceivableRepo(pool)
		dunningRunSvc := kasse.NewDunningRunService(dunningRepo, dunningRepo, receivableRepo, dunningSvc, store)
		artefactStore, storeErr := storage.FromEnv(ctx, strings.TrimSpace(os.Getenv("ERP_DOCUMENTS_DIR")))
		if storeErr != nil {
			log.Fatalf("api: artefact store: %v", storeErr)
		}
		// The API role queues work for the worker role; it never works a job itself
		// (docs/17, docs/27). River owns its own schema, so apply it before inserting.
		if err := jobs.Migrate(ctx, pool); err != nil {
			log.Fatalf("api: jobs: %v", err)
		}
		enqueuer, err := jobs.NewEnqueuer(pool)
		if err != nil {
			log.Fatalf("api: job enqueuer: %v", err)
		}
		// The report catalogue is the same one the worker renders from, so the reports this API
		// offers and the reports that actually render cannot drift apart (docs/25).
		klrSvc := klr.NewService(klr.NewRepo(pool), store)
		reportCatalogue := reports.NewCatalogue(reports.DataSources{
			Bookings:  kasse.NewLedgerRepo(pool),
			Positions: budgetSvc,
			Centres:   klrSvc,
			Units:     klrSvc,
		})
		// The e-invoice service matches an inbound invoice to a purchase order, so it needs the
		// procurement service (which owns the order and the budget conversion).
		procurementSvc := procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store)
		deps := httpapi.Deps{
			Kasse:           kasse.NewService(kasse.NewRepo(pool), store),
			Orders:          kasse.NewOrderService(kasse.NewOrderRepo(pool), budgetSvc, store),
			Dunning:         dunningSvc,
			DunningRun:      dunningRunSvc,
			Artefacts:       artefactStore,
			Tax:             kasse.NewTaxService(kasse.NewTaxRepo(pool), store),
			Runs:            kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store),
			Statements:      kasse.NewStatementService(kasse.NewStatementRepo(pool), store),
			FoundationBooks: kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store),
			EInvoices:       einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), procurementSvc, store),
			Budget:          budgetSvc,
			Lookups:         lookupSvc,
			Procurement:     procurementSvc,
			KLR:             klrSvc,
			Mandates:        kasse.NewMandateService(kasse.NewMandateRepo(pool), store),
			Receivables:     kasse.NewReceivableService(receivableRepo, store),
			CollectionRuns:  kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), receivableRepo, store),
			ETL:             etlRegistry,
			Audit:           store,
			Health:          pool,
			Exports:         jobs.NewExports(enqueuer),
			Reports:         reportCatalogue,
		}
		if cfg.OIDCJWKSURL != "" {
			deps.Auth = &auth.JWKVerifier{
				Keys:     &auth.RemoteKeySet{URL: cfg.OIDCJWKSURL, TTL: time.Hour},
				Issuer:   cfg.OIDCIssuer,
				Audience: cfg.OIDCAudience,
			}
		}
		return httpapi.New(deps)
	}

	var handler http.Handler
	if cfg.TenantDSN != "" {
		// Multi-tenant: one database per tenant, resolved from the request host
		// (ADR-0002). The tenant's pool is opened and migrated on first request.
		reg, err := tenant.NewRegistry(cfg.TenantDSN, db.Open, db.Migrate)
		if err != nil {
			log.Fatalf("tenant: %v", err)
		}
		defer reg.Close()
		handler = httpapi.NewTenancy(func(ctx context.Context, name string) (http.Handler, error) {
			pool, err := reg.Pool(ctx, name)
			if err != nil {
				return nil, err
			}
			return buildServer(pool).Routes(), nil
		}).Handler()
		log.Printf("api listening on %s (multi-tenant, role=%q)", cfg.HTTPAddr, cfg.Role)
	} else {
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
		handler = buildServer(pool).Handler()
		log.Printf("api listening on %s (tenant=%q role=%q)", cfg.HTTPAddr, cfg.Tenant, cfg.Role)
	}

	if cfg.OIDCJWKSURL == "" {
		log.Printf("auth: DISABLED (set ERP_OIDC_JWKS_URL to require tokens)")
	} else {
		log.Printf("auth: OIDC enabled (issuer=%q)", cfg.OIDCIssuer)
	}

	if err := http.ListenAndServe(cfg.HTTPAddr, handler); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// dunningDocumentsFromEnv builds the dunning document issuer, or nil when the document
// service or the store directory is not configured: notices are then issued without a
// document rather than failing.
func dunningDocumentsFromEnv() kasse.DunningDocuments {
	url := strings.TrimSpace(os.Getenv("ERP_DOCUMENTS_URL"))
	dir := strings.TrimSpace(os.Getenv("ERP_DOCUMENTS_DIR"))
	if url == "" || dir == "" {
		return nil
	}
	renderer, err := documents.NewHTTPRenderer(url, nil)
	if err != nil {
		log.Printf("documents: %v - notices will have no document", err)
		return nil
	}
	store, err := storage.FromEnv(context.Background(), dir)
	if err != nil {
		log.Printf("documents: %v - notices will have no document", err)
		return nil
	}
	return dunningDocuments{renderer: renderer, store: store, issuer: organisation()}
}

func organisation() string {
	if v := strings.TrimSpace(os.Getenv("ERP_ORGANISATION")); v != "" {
		return v
	}
	return "Universitaet"
}

// dunningDocuments renders a dunning notice and stores the PDF.
type dunningDocuments struct {
	renderer documents.Renderer
	store    documents.Store
	issuer   string
}

// StoreNotice implements kasse.DunningDocuments.
func (d dunningDocuments) StoreNotice(ctx context.Context, n kasse.DunningNotice, level kasse.DunningLevel) (string, error) {
	// The claim reference stands in for the addressee until claims carry a debtor.
	html, err := (documents.DunningNotice{
		Reference:   n.ClaimReference,
		DebtorName:  n.ClaimReference,
		Issuer:      d.issuer,
		Level:       dunningLevelNumber(n.LevelCode),
		AmountCents: n.OutstandingCents + n.ChargeCents,
		Currency:    "EUR",
		DueDate:     n.IssuedOn.AddDate(0, 0, level.DaysAllowed).Format("2006-01-02"),
	}).HTML()
	if err != nil {
		return "", err
	}
	pdf, err := d.renderer.Render(ctx, html)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("dunning/%d.pdf", n.ID)
	if err := d.store.Save(ctx, key, pdf); err != nil {
		return "", err
	}
	return key, nil
}

// dunningLevelNumber reads the numeric stage out of a level code, defaulting to 1.
func dunningLevelNumber(code string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(code)); err == nil && n > 0 {
		return n
	}
	return 1
}
