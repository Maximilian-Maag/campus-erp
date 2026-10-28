package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	principalKey
)

// RequestID returns the correlation id attached to a request, or "" if none.
// Handlers include it in error envelopes so a client report maps to a log line
// and an audit event (NFR-5).
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// middlware wraps a handler with the cross-cutting concerns every request needs:
// a correlation id, panic recovery and a structured access log (NFR-5). Keeping
// them here means no handler repeats them and the transport layer stays thin.
func middleware(h http.Handler, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		w.Header().Set("X-Request-ID", id)

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", "request_id", id, "path", r.URL.Path, "err", rec)
				if !sw.wrote {
					writeProblem(sw, http.StatusInternalServerError, "internal error")
				}
			}
			log.Info("request",
				"method", r.Method, "path", r.URL.Path,
				"status", sw.status, "duration_ms", time.Since(start).Milliseconds(),
				"request_id", id)
		}()
		h.ServeHTTP(sw, r)
	})
}

// statusWriter records the status code and whether the body was written, so the
// access log is accurate and a panic after a partial write is not double-served.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// writeProblem writes the shared error envelope (components/schemas/Problem in
// contracts/openapi.yaml) so every error has one shape the client can parse.
func writeProblem(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
