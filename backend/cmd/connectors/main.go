// Command connectors hosts native-dependency integrations (role: connectors):
// ELSTER via ERiC (cgo) and EBICS banking. Kept out of the core image (ADR-0005).
package main

import (
	"log"

	"github.com/Maximilian-Maag/hisinone-erp/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("connectors starting (tenant=%q role=%q)", cfg.Tenant, cfg.Role)
	// TODO: ERiC (cgo) and EBICS adapters; consume jobs from the core over the
	// internal contract (queue + object storage).
	select {}
}
