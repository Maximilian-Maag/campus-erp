// Command connectors hosts native-dependency integrations (role: connectors):
// ELSTER via ERiC (cgo) and EBICS banking. Kept out of the core image (ADR-0005).
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("connectors starting (tenant=%q role=%q)", cfg.Tenant, cfg.Role)
	// TODO: ERiC (cgo) and EBICS adapters; consume jobs from the core over the
	// internal contract (queue + object storage).
	//
	// Wait on a signal rather than an empty select: the Go runtime treats a bare
	// `select {}` in the only goroutine as a deadlock ("all goroutines are asleep")
	// and aborts the process, so the role crash-looped instead of staying up.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Printf("connectors stopping")
}
