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

// TestProblemEnvelopeCarriesCodeAndRequestID: the envelope the spec documents is the one
// the server sends — the machine code for the status, and the correlation id the client
// also gets in X-Request-ID.
func TestProblemEnvelopeCarriesCodeAndRequestID(t *testing.T) {
	h := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeProblem(w, http.StatusNotFound, "no such thing")
	}), slog.Default())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a Problem: %v (%q)", err, rec.Body.String())
	}
	if p.Error != "no such thing" {
		t.Errorf("error = %q, want the message", p.Error)
	}
	if p.Code != "not_found" {
		t.Errorf("code = %q, want not_found", p.Code)
	}
	if hdr := rec.Header().Get("X-Request-ID"); p.RequestID == "" || p.RequestID != hdr {
		t.Errorf("request_id = %q, header = %q; want the same non-empty id", p.RequestID, hdr)
	}
}

// TestWriteProblemCodeUsesTheGivenCode: an explicit code is written verbatim, for errors a
// client branches on rather than matches by text.
func TestWriteProblemCodeUsesTheGivenCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeProblemCode(rec, http.StatusUnprocessableEntity, "state_required", "state is required")
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a Problem: %v", err)
	}
	if p.Code != "state_required" {
		t.Errorf("code = %q, want state_required", p.Code)
	}
	// Written to a bare recorder (no middleware wrapper) the envelope simply has no id.
	if p.RequestID != "" {
		t.Errorf("request_id = %q, want empty without a wrapper", p.RequestID)
	}
}

func TestCodeForStatus(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:          "bad_request",
		http.StatusUnauthorized:        "unauthorized",
		http.StatusForbidden:           "forbidden",
		http.StatusNotFound:            "not_found",
		http.StatusConflict:            "conflict",
		http.StatusUnprocessableEntity: "unprocessable",
		http.StatusServiceUnavailable:  "unavailable",
		http.StatusInternalServerError: "internal",
		http.StatusTeapot:              "error", // anything unmapped
	}
	for status, want := range cases {
		if got := codeForStatus(status); got != want {
			t.Errorf("codeForStatus(%d) = %q, want %q", status, got, want)
		}
	}
}
