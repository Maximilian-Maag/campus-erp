// Command api is the HTTP entrypoint (role: api).
package main

import (
	"context"
	"log"
	"net/http"
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
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()

	// buildServer wires the whole modular monolith over one tenant's pool. It is
	// the seam the multi-tenant front uses to build each tenant's server lazily.
	buildServer := func(pool *pgxpool.Pool) *httpapi.Server {
		store := audit.NewStore(pool)
		budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
		deps := httpapi.Deps{
			Kasse:           kasse.NewService(kasse.NewRepo(pool), store),
			Orders:          kasse.NewOrderService(kasse.NewOrderRepo(pool), store),
			Dunning:         kasse.NewDunningService(kasse.NewDunningRepo(pool), store),
			Tax:             kasse.NewTaxService(kasse.NewTaxRepo(pool), store),
			Runs:            kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store),
			Statements:      kasse.NewStatementService(kasse.NewStatementRepo(pool), store),
			FoundationBooks: kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store),
			EInvoices:       einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), store),
			Budget:          budgetSvc,
			Lookups:         lookup.NewValueService(lookup.NewValueRepo(pool), store),
			Procurement:     procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store),
			KLR:             klr.NewService(klr.NewRepo(pool), store),
			Mandates:        kasse.NewMandateService(kasse.NewMandateRepo(pool), store),
			Audit:           store,
			Health:          pool,
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
