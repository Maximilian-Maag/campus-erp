package db_test

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

func TestMigrationsAreOrderedAndVersioned(t *testing.T) {
	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations are embedded")
	}
	for i, m := range ms {
		if m.Version <= 0 {
			t.Errorf("migration %d (%s) has version %d", i, m.Name, m.Version)
		}
		if m.SQL == "" {
			t.Errorf("migration %s has empty SQL", m.Name)
		}
		if i > 0 && ms[i-1].Version >= m.Version {
			t.Errorf("migrations not strictly ordered: %d then %d", ms[i-1].Version, m.Version)
		}
	}
}

func TestOpenRejectsEmptyDSN(t *testing.T) {
	if _, err := db.Open(context.Background(), ""); err == nil {
		t.Fatal("an empty DSN must be rejected")
	}
}

func TestOpenReportsUnreachableServer(t *testing.T) {
	// Port 1 is not a Postgres server; Open must fail rather than return a pool.
	if _, err := db.Open(context.Background(), "postgres://postgres:x@127.0.0.1:1/nope"); err == nil {
		t.Fatal("an unreachable server must be reported")
	}
}

func TestDSNFromEnv(t *testing.T) {
	t.Setenv(db.EnvDatabaseURL, "")
	if _, err := db.DSNFromEnv(); err == nil {
		t.Fatal("a missing DATABASE_URL must be an error")
	}
	t.Setenv(db.EnvDatabaseURL, "postgres://example")
	got, err := db.DSNFromEnv()
	if err != nil {
		t.Fatalf("DSNFromEnv: %v", err)
	}
	if got != "postgres://example" {
		t.Fatalf("got %q", got)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	applied, err := db.Migrate(ctx, pool)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if applied != 0 {
		t.Fatalf("second Migrate applied %d migrations, want 0", applied)
	}
}

func TestMigrationsFromSkipsDirsAndNonSQL(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0002_second.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		"migrations/0001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
		"migrations/notes.txt":       &fstest.MapFile{Data: []byte("ignore me")},
		"migrations/nested":          &fstest.MapFile{Mode: fs.ModeDir},
	}
	ms, err := db.MigrationsFrom(fsys)
	if err != nil {
		t.Fatalf("MigrationsFrom: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("got %d migrations, want 2 (dirs and non-.sql are skipped)", len(ms))
	}
	if ms[0].Version != 1 || ms[1].Version != 2 {
		t.Fatalf("migrations are not ordered: %+v", ms)
	}
	if ms[0].Name != "0001_first.sql" || ms[0].SQL != "SELECT 1;" {
		t.Fatalf("first migration wrong: %+v", ms[0])
	}
}

func TestMigrationsFromRejectsBadNames(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"no underscore":       {"migrations/bad.sql": &fstest.MapFile{Data: []byte("x")}},
		"non-numeric version": {"migrations/xx_a.sql": &fstest.MapFile{Data: []byte("x")}},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := db.MigrationsFrom(fsys); err == nil {
				t.Fatal("a malformed migration name must be an error")
			}
		})
	}
}

func TestMigrationsFromMissingDirectory(t *testing.T) {
	if _, err := db.MigrationsFrom(fstest.MapFS{}); err == nil {
		t.Fatal("a missing migrations directory must be an error")
	}
}

func TestMigrationsFromRejectsDuplicateVersions(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0001_a.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"migrations/0001_b.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
	}
	if _, err := db.MigrationsFrom(fsys); err == nil {
		t.Fatal("duplicate migration versions must be an error")
	}
}
