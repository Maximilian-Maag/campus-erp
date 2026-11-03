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

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK, reqID: id}
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
// access log is accurate and a panic after a partial write is not double-served. It also
// carries the correlation id, so a handler that is handed only the ResponseWriter can
// still stamp it onto an error envelope.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
	reqID  string
}

// requestID is the correlation id of the request this writer belongs to.
func (w *statusWriter) requestID() string { return w.reqID }

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Problem is the shared error envelope (components/schemas/Problem in
// contracts/openapi.yaml): every error a client sees has one shape to parse.
type Problem struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// requestIDCarrier is implemented by the wrapper the middleware installs, so an error
// envelope can carry the correlation id without threading the request through every
// handler.
type requestIDCarrier interface{ requestID() string }

// writeProblem writes the shared error envelope with the machine code that belongs to the
// status. Handlers keep calling it with three arguments, so the envelope is uniform by
// construction rather than by discipline.
func writeProblem(w http.ResponseWriter, status int, msg string) {
	writeProblemCode(w, status, codeForStatus(status), msg)
}

// writeProblemCode writes the envelope with an explicit machine code, for an error a
// client should branch on rather than match by its text.
func writeProblemCode(w http.ResponseWriter, status int, code, msg string) {
	p := Problem{Error: msg, Code: code}
	if c, ok := w.(requestIDCarrier); ok {
		p.RequestID = c.requestID()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// codeForStatus is the stable machine code for a status, so a client can branch on it. It
// never varies with the message.
func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "unprocessable"
	case http.StatusServiceUnavailable:
		return "unavailable"
	case http.StatusInternalServerError:
		return "internal"
	default:
		return "error"
	}
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
