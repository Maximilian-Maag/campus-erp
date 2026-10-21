package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

// TestMigrateAppliesToFreshDatabase creates a throwaway database, applies the
// embedded migrations to it, and asserts they are applied exactly once. The shared
// test database is usually already migrated, so the "applied" path (and the
// counter increment) would otherwise never be covered.
func TestMigrateAppliesToFreshDatabase(t *testing.T) {
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	admin, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer admin.Close()

	name := fmt.Sprintf("campus_erp_migtest_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("CREATE DATABASE: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name)
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	cfg.ConnConfig.Database = name
	fresh, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to %s: %v", name, err)
	}
	defer fresh.Close()

	applied, err := db.Migrate(ctx, fresh)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	if applied != len(ms) {
		t.Fatalf("a fresh database applied %d migrations, want %d", applied, len(ms))
	}
	again, err := db.Migrate(ctx, fresh)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if again != 0 {
		t.Fatalf("second Migrate applied %d migrations, want 0", again)
	}
}
