package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

// Tenancy is the multi-tenant front for a set of per-tenant servers (ADR-0002).
// It resolves the tenant from the request, builds (and caches) that tenant's
// handler, and delegates. A malformed or unknown host is rejected before any
// tenant code runs, so no request can reach the wrong tenant's data.
type Tenancy struct {
	// Resolve derives the tenant name from a request. Defaults to the first DNS
	// label of the Host header (with an X-Tenant override for local testing).
	Resolve func(*http.Request) (string, error)
	// Build returns the handler for a tenant, creating its services over that
	// tenant's database pool. Called once per tenant and cached.
	Build func(ctx context.Context, name string) (http.Handler, error)

	Log *slog.Logger

	mu      sync.Mutex
	servers map[string]http.Handler
}

// NewTenancy returns a Tenancy with the default resolver.
func NewTenancy(build func(context.Context, string) (http.Handler, error)) *Tenancy {
	return &Tenancy{Build: build, servers: map[string]http.Handler{}}
}

// Handler returns the multi-tenant HTTP handler.
func (t *Tenancy) Handler() http.Handler {
	return middleware(http.HandlerFunc(t.serve), t.Log)
}

func (t *Tenancy) serve(w http.ResponseWriter, r *http.Request) {
	name, err := t.resolve(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "unknown tenant")
		return
	}
	h, err := t.handlerFor(r.Context(), name)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "tenant unavailable")
		return
	}
	r = r.WithContext(tenant.WithTenant(r.Context(), name))
	h.ServeHTTP(w, r)
}

func (t *Tenancy) resolve(r *http.Request) (string, error) {
	if t.Resolve != nil {
		return t.Resolve(r)
	}
	if h := r.Header.Get("X-Tenant"); h != "" {
		return tenant.Resolve(h)
	}
	return tenant.Resolve(r.Host)
}

func (t *Tenancy) handlerFor(ctx context.Context, name string) (http.Handler, error) {
	t.mu.Lock()
	if h, ok := t.servers[name]; ok {
		t.mu.Unlock()
		return h, nil
	}
	t.mu.Unlock()

	h, err := t.Build(ctx, name)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.servers[name]; ok {
		return existing, nil // another goroutine won the race
	}
	t.servers[name] = h
	return h, nil
}
