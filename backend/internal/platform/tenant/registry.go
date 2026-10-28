package tenant

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

// OpenFunc opens a pool for a tenant DSN.
type OpenFunc func(ctx context.Context, dsn string) (*pgxpool.Pool, error)

// MigrateFunc applies migrations to a tenant pool and returns how many ran.
type MigrateFunc func(ctx context.Context, pool *pgxpool.Pool) (int, error)

// Registry hands out one *pgxpool.Pool per tenant, opened lazily on first use and
// migrated once. This is the database-per-tenant seam (ADR-0002): the DSN is a
// template whose {tenant} placeholder is filled with the validated tenant name,
// so a tenant can never point at another tenant's database.
type Registry struct {
	dsn     string
	open    OpenFunc
	migrate MigrateFunc

	mu    sync.Mutex
	pools map[string]*pgxpool.Pool
}

// NewRegistry builds a Registry. dsn must contain the "{tenant}" placeholder.
func NewRegistry(dsn string, open OpenFunc, migrate MigrateFunc) (*Registry, error) {
	if !strings.Contains(dsn, "{tenant}") {
		return nil, fmt.Errorf("tenant: dsn template must contain {tenant}")
	}
	if open == nil {
		open = db.Open
	}
	if migrate == nil {
		migrate = db.Migrate
	}
	return &Registry{dsn: dsn, open: open, migrate: migrate, pools: map[string]*pgxpool.Pool{}}, nil
}

// Pool returns the tenant's pool, opening and migrating it on first use.
func (r *Registry) Pool(ctx context.Context, name string) (*pgxpool.Pool, error) {
	if !label.MatchString(name) {
		return nil, fmt.Errorf("tenant: invalid tenant name %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.pools[name]; ok {
		return p, nil
	}
	dsn := strings.ReplaceAll(r.dsn, "{tenant}", name)
	pool, err := r.open(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("tenant %q: open: %w", name, err)
	}
	if _, err := r.migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("tenant %q: migrate: %w", name, err)
	}
	r.pools[name] = pool
	return pool, nil
}

// Close closes every pool the registry opened.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, p := range r.pools {
		p.Close()
		delete(r.pools, name)
	}
}
