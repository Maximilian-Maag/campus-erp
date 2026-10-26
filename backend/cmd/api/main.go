// Command api is the HTTP entrypoint (role: api).
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()

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

	store := audit.NewStore(pool)
	api := httpapi.New(
		kasse.NewService(kasse.NewRepo(pool), store),
		kasse.NewOrderService(kasse.NewOrderRepo(pool), store),
		kasse.NewDunningService(kasse.NewDunningRepo(pool), store),
		kasse.NewTaxService(kasse.NewTaxRepo(pool), store),
		kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store),
		store,
	)

	log.Printf("api listening on %s (tenant=%q role=%q)", cfg.HTTPAddr, cfg.Tenant, cfg.Role)
	if err := http.ListenAndServe(cfg.HTTPAddr, api.Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
