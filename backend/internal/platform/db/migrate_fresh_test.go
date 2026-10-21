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

// TestMigrateIsConcurrencySafe reproduces the collision two migrators hit when they
// run at the same time: CREATE TABLE IF NOT EXISTS is not race-free, so without the
// advisory lock this fails with a pg_type duplicate-key error.
func TestMigrateIsConcurrencySafe(t *testing.T) {
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

	name := fmt.Sprintf("campus_erp_racetest_%d", time.Now().UnixNano())
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
		t.Fatalf("connect: %v", err)
	}
	defer fresh.Close()

	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}

	const racers = 4
	errs := make(chan error, racers)
	applied := make(chan int, racers)
	for i := 0; i < racers; i++ {
		go func() {
			n, err := db.Migrate(ctx, fresh)
			errs <- err
			applied <- n
		}()
	}
	total := 0
	for i := 0; i < racers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Migrate failed: %v", err)
		}
		total += <-applied
	}
	if total != len(ms) {
		t.Fatalf("migrations applied in total = %d, want exactly %d (each migration once)", total, len(ms))
	}
}
