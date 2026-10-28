package tenant

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

// TestRegistryAgainstRealDatabases proves database-per-tenant for real: two
// tenant databases are created, migrated through the registry, and each ends up
// with its own migrated schema. Skipped unless DATABASE_URL points at a server.
func TestRegistryAgainstRealDatabases(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	// Names must be valid DNS labels (the registry validates them) *and*
	// valid database names, so no underscores.
	names := []string{"ittenant1", "ittenant2"}
	for _, n := range names {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+n)
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+n); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	t.Cleanup(func() {
		for _, n := range names {
			_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+n)
		}
	})

	template, err := dsnWithDatabase(dsn, "{tenant}")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	reg, err := NewRegistry(template, db.Open, db.Migrate)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	defer reg.Close()

	for _, n := range names {
		pool, err := reg.Pool(ctx, n)
		if err != nil {
			t.Fatalf("pool %s: %v", n, err)
		}
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='audit_event')",
		).Scan(&exists); err != nil {
			t.Fatalf("query %s: %v", n, err)
		}
		if !exists {
			t.Errorf("tenant %s: audit_event table missing after migrate", n)
		}
	}
}

// dsnWithDatabase rewrites the database name in a postgres DSN without
// URL-encoding the "{tenant}" placeholder.
func dsnWithDatabase(dsn, database string) (string, error) {
	base, query, _ := strings.Cut(dsn, "?")
	i := strings.LastIndex(base, "/")
	if i < 0 {
		return "", fmt.Errorf("dsn: no database path in %q", dsn)
	}
	out := base[:i+1] + database
	if query != "" {
		out += "?" + query
	}
	return out, nil
}
