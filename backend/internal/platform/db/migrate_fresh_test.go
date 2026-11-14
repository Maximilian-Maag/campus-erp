package db_test

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// TestDroppingASiblingSchemaKeepsTheExclusionConstraints is the regression test for the flake in
// issues #136 and #138: two tests that assert "a second, overlapping allocation run is refused"
// failed with `second run = 201`, and the schema guard reported the exclusion constraints missing on
// a schema that had just been migrated. Neither was about the constraints: the *extension* they
// depend on had been installed into a per-process schema, and dropping that schema — which is what a
// test process does when it exits — takes btree_gist and every exclusion constraint that needs it,
// in every other schema.
//
// Two migrators with separate search_paths, the way internal/platform/dbtest runs them, each apply
// the migrations; then the first one's schema is dropped. The second schema's constraints must
// survive, and the extension must not be in a per-process schema in the first place.
func TestDroppingASiblingSchemaKeepsTheExclusionConstraints(t *testing.T) {
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

	name := fmt.Sprintf("campus_erp_gisttest_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("CREATE DATABASE: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name)
	})

	for _, schema := range []string{"test_first", "test_second"} {
		pool := poolOn(t, ctx, dsn, name, schema)
		if _, err := db.Migrate(ctx, pool); err != nil {
			pool.Close()
			t.Fatalf("Migrate into %s: %v", schema, err)
		}
		pool.Close()
	}

	// The first migrator's process exits, which the harness does with DROP SCHEMA ... CASCADE. It has
	// to run on a connection to the throwaway database: `admin` is connected to the one the throwaway
	// was created from.
	killer := poolOn(t, ctx, dsn, name, "")
	if _, err := killer.Exec(ctx, "DROP SCHEMA test_first CASCADE"); err != nil {
		killer.Close()
		t.Fatalf("drop the sibling schema: %v", err)
	}
	killer.Close()

	second := poolOn(t, ctx, dsn, name, "test_second")
	defer second.Close()

	var extSchema string
	if err := second.QueryRow(ctx,
		`SELECT n.nspname
		   FROM pg_extension e
		   JOIN pg_namespace n ON n.oid = e.extnamespace
		  WHERE e.extname = 'btree_gist'`).Scan(&extSchema); err != nil {
		t.Fatalf("read btree_gist's schema: %v", err)
	}
	if strings.HasPrefix(extSchema, "test_") {
		t.Fatalf("btree_gist is installed in %q, a schema that is dropped when its process exits; "+
			"dropping it takes every exclusion constraint that depends on it, in every schema "+
			"(issues #136, #138)", extSchema)
	}

	var present int
	if err := second.QueryRow(ctx,
		`SELECT count(*)
		   FROM pg_constraint c
		   JOIN pg_class t ON t.oid = c.conrelid
		   JOIN pg_namespace n ON n.oid = t.relnamespace
		  WHERE n.nspname = current_schema()
		    AND c.contype = 'x'
		    AND c.conname = ANY($1)`,
		[]string{"allocation_run_no_overlap", "cost_unit_run_no_overlap"}).Scan(&present); err != nil {
		t.Fatalf("read the exclusion constraints: %v", err)
	}
	if present != 2 {
		t.Fatalf("after a sibling schema was dropped, %d of the 2 exclusion constraints are left in "+
			"the schema that stayed — a duplicate the database must refuse would be accepted", present)
	}
}

// poolOn opens a pool on one database with its search_path set to one schema, creating the schema
// first. An empty schema name leaves the DSN's own search_path alone. It is the migrator setup the
// test harness uses, without importing the harness (which imports this package).
func poolOn(t *testing.T, ctx context.Context, dsn, database, schema string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	cfg.ConnConfig.Database = database
	if schema != "" {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to %s: %v", database, err)
	}
	if schema != "" {
		if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
			pool.Close()
			t.Fatalf("CREATE SCHEMA %s: %v", schema, err)
		}
	}
	return pool
}
