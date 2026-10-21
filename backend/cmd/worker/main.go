// Command worker runs asynchronous jobs (role: worker): imports, exports, ETL,
// report generation. Queue backend is River on Postgres (ADR-0007); not yet wired.
package main

import (
	"log"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("worker starting (tenant=%q role=%q)", cfg.Tenant, cfg.Role)
	// TODO: connect River, register job kinds (etl, sepa-export, mt940-import,
	// statistics-export, report-render), then block.
	select {}
}
