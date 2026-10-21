// Command api is the HTTP entrypoint (role: api).
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logbook := &audit.Log{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Demonstration endpoint for the append-only audit stream (ADR-0006).
	mux.HandleFunc("POST /api/audit", func(w http.ResponseWriter, r *http.Request) {
		var e audit.Event
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(logbook.Append(e))
	})

	mux.HandleFunc("GET /api/audit/verify", func(w http.ResponseWriter, _ *http.Request) {
		if err := logbook.Verify(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("api listening on %s (tenant=%q role=%q)", cfg.HTTPAddr, cfg.Tenant, cfg.Role)
	if err := http.ListenAndServe(cfg.HTTPAddr, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
