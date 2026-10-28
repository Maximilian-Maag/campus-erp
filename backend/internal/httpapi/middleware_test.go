package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareAssignsRequestID(t *testing.T) {
	h := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestID(r.Context()) == "" {
			t.Error("handler saw no request id")
		}
		w.WriteHeader(http.StatusNoContent)
	}), slog.Default())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("response is missing X-Request-ID")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestMiddlewareKeepsIncomingRequestID(t *testing.T) {
	h := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Errorf("X-Request-ID = %q, want the incoming value", got)
	}
}

func TestMiddlewareRecoversFromPanic(t *testing.T) {
	h := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), slog.Default())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, rec.Body.String())
	}
	if body["error"] == "" {
		t.Error("error envelope is empty")
	}
}

func TestMiddlewareToleratesNilLogger(t *testing.T) {
	h := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestStatusWriterRecordsStatus(t *testing.T) {
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	sw.WriteHeader(http.StatusTeapot)
	if sw.status != http.StatusTeapot || !sw.wrote {
		t.Errorf("statusWriter status=%d wrote=%v", sw.status, sw.wrote)
	}
}
