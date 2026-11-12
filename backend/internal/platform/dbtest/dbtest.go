// Package dbtest opens an isolated database pool for integration tests.
//
// The test database is shared by every test binary on a machine, and Go runs
// packages — and gremlins runs a package's tests — in parallel. Two processes
// sharing the same tables race: a fixture one process writes (say a payment
// recipient keyed by a fixed IBAN) can be overwritten by another, which shows up
// as a test that fails once in a while and passes in isolation.
//
// The fix is isolation, not more unique suffixes: each test *process* gets its
// own schema. The pool's search_path points at it and the migrations run into it,
// so no two processes ever share a table. Logical requirement of the harness —
// all queries must be unqualified (which the migrations and repos are).
package dbtest

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

var (
	initOnce sync.Once
	schema   string
	initErr  error
)

// Pool returns a pool whose search_path is a schema private to this test process,
// with migrations applied. It skips (or fails, in CI) when DATABASE_URL is unset.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	initOnce.Do(func() { schema, initErr = ensureSchema(ctx, dsn) })
	if initErr != nil {
		t.Fatalf("dbtest: %v", initErr)
	}
	pool, err := openInSchema(ctx, dsn, schema)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("dbtest: migrate: %v", err)
	}
	// A schema that is missing an exclusion constraint the migrations should have added fails here,
	// located and named, instead of as a duplicate accepted in some unrelated assertion (issue #136).
	if err := verifySchema(ctx, pool, requiredConstraints); err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	return pool
}

// Schema returns this test process's schema name.
func Schema() string { return schema }

// Main runs a package's tests and drops this process's schema afterwards, so the
// shared development database does not accumulate test schemas. Use it from a
// package's TestMain: func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }.
func Main(m *testing.M) int {
	code := m.Run()
	drop()
	return code
}

func drop() {
	if schema == "" {
		return
	}
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		return
	}
	ctx := context.Background()
	base, err := db.Open(ctx, dsn)
	if err != nil {
		return
	}
	defer base.Close()
	_, _ = base.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
}

// ensureSchema creates a fresh schema private to this process. The name is
// derived from the PID, so a process that reuses a PID from an earlier run starts
// clean.
func ensureSchema(ctx context.Context, dsn string) (string, error) {
	base, err := db.Open(ctx, dsn)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer base.Close()
	cleanupStale(ctx, base)
	name := fmt.Sprintf("test_%d", os.Getpid())
	if _, err := base.Exec(ctx, "DROP SCHEMA IF EXISTS "+name+" CASCADE"); err != nil {
		return "", fmt.Errorf("drop schema: %w", err)
	}
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		return "", fmt.Errorf("create schema: %w", err)
	}
	return name, nil
}

// cleanupStale drops schemas left behind by test processes that are gone. A
// process that is killed (gremlins kills timed-out test runs) never reaches
// TestMain, so relying on exit cleanup alone leaks schemas. Only processes whose
// PID no longer exists are dropped, so a concurrently running test is safe. This
// needs /proc (Linux); elsewhere it is skipped, since liveness is unknowable.
func cleanupStale(ctx context.Context, base *pgxpool.Pool) {
	if _, err := os.Stat("/proc"); err != nil {
		return
	}
	rows, err := base.Query(ctx, "SELECT nspname FROM pg_namespace WHERE nspname LIKE 'test_%'")
	if err != nil {
		return
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			names = append(names, n)
		}
	}
	rows.Close()
	self := os.Getpid()
	for _, n := range names {
		pid, err := strconv.Atoi(strings.TrimPrefix(n, "test_"))
		if err != nil || pid == self || processAlive(pid) {
			continue
		}
		_, _ = base.Exec(ctx, "DROP SCHEMA IF EXISTS "+n+" CASCADE")
	}
}

func processAlive(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func openInSchema(ctx context.Context, dsn, schema string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
