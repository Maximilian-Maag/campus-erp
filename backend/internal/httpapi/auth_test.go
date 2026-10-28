package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
)

// echoActor is a stand-in for any handler that records the caller.
func echoActor(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(actor(r)))
}

func TestAuthRejectsMissingToken(t *testing.T) {
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{"good": {Subject: "alice"}}}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthRejectsBadToken(t *testing.T) {
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{"good": {Subject: "alice"}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/audit/verify", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthAcceptsTokenAndSetsActor(t *testing.T) {
	// Exercise the real auth middleware over a handler that echoes the actor, so
	// we can assert the token subject propagates into the request.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/whoami", echoActor)
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{"good": {Subject: "alice"}}}})
	handler := srv.authenticate(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "alice" {
		t.Fatalf("actor = %q, want alice", rec.Body.String())
	}
}

func TestAuthLeavesProbesOpen(t *testing.T) {
	srv := New(Deps{Auth: auth.StaticVerifier{Tokens: map[string]auth.Principal{}}})
	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s should not require a token", path)
		}
	}
}

func TestActorFallsBackToHeaderWhenNoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if actor(req) != "anonymous" {
		t.Errorf("actor = %q, want anonymous", actor(req))
	}
	req.Header.Set("X-Actor", "dev")
	if actor(req) != "dev" {
		t.Errorf("actor = %q, want dev", actor(req))
	}
	cases := map[string]string{
		"":           "",
		"Bearer xyz": "xyz",
		"bearer xyz": "xyz",
		"Basic xyz":  "",
		"Bearer":     "",
		"Bearer a b": "",
	}
	for header, want := range cases {
		req.Header.Set("Authorization", header)
		if got := bearerToken(req); got != want {
			t.Errorf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestReadyTimeoutIsTwoSeconds(t *testing.T) {
	if readyTimeout != 2*time.Second {
		t.Errorf("readyTimeout = %v, want 2s", readyTimeout)
	}
}
