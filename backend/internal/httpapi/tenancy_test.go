package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

func TestTenancyDispatchesAndCaches(t *testing.T) {
	var builds int64
	tr := NewTenancy(func(_ context.Context, name string) (http.Handler, error) {
		atomic.AddInt64(&builds, 1)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, _ := tenant.From(r.Context())
			_, _ = w.Write([]byte("tenant=" + got))
		}), nil
	})

	for i := 0; i < 2; i++ { // two requests, one tenant -> built once
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Host = "uni-x.erp.example.org"
		rec := httptest.NewRecorder()
		tr.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "tenant=uni-x" {
			t.Fatalf("resp = %d %q", rec.Code, rec.Body.String())
		}
	}
	if builds != 1 {
		t.Errorf("builds = %d, want 1 (cached)", builds)
	}
}

func TestTenancyRejectsUnknownHost(t *testing.T) {
	tr := NewTenancy(func(context.Context, string) (http.Handler, error) { return http.NotFoundHandler(), nil })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "-bad-.example.org"
	rec := httptest.NewRecorder()
	tr.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestTenancyBuildFailureIsUnavailable(t *testing.T) {
	tr := NewTenancy(func(context.Context, string) (http.Handler, error) { return nil, errors.New("db down") })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "uni-x.erp.example.org"
	rec := httptest.NewRecorder()
	tr.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestTenancyHeaderOverride(t *testing.T) {
	tr := NewTenancy(func(_ context.Context, name string) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(name)) }), nil
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "ignored.example.org"
	req.Header.Set("X-Tenant", "uni-y")
	rec := httptest.NewRecorder()
	tr.Handler().ServeHTTP(rec, req)
	if rec.Body.String() != "uni-y" {
		t.Fatalf("body = %q, want uni-y", rec.Body.String())
	}
}

// TestTenancyQualifiesTheActor: the tenant the front resolves is the one the actor an
// audit event would carry is qualified with, so the two cannot drift apart.
func TestTenancyQualifiesTheActor(t *testing.T) {
	var got string
	tr := NewTenancy(func(_ context.Context, _ string) (http.Handler, error) {
		return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = actor(r) }), nil
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Host = "uni-x.erp.example.org"
	req.Header.Set("X-Actor", "alice")
	tr.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if got != "uni-x/alice" {
		t.Fatalf("actor through the tenancy front = %q, want uni-x/alice", got)
	}
}
