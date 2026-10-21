package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationTable = `CREATE TABLE IF NOT EXISTS schema_migration (
	version    bigint      PRIMARY KEY,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migration is one ordered schema change.
type Migration struct {
	Version int64
	Name    string
	SQL     string
}

// Migrations returns the embedded migrations, ordered by version.
func Migrations() ([]Migration, error) { return MigrationsFrom(migrationsFS) }

// MigrationsFrom loads migrations from any filesystem laid out as
// <dir>/<version>_<name>.sql. Directories and non-.sql files are ignored; a
// malformed filename is an error. Taking an fs.FS makes the skip and error
// branches testable.
func MigrationsFrom(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("db: read migrations: %w", err)
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("db: migration %q must be named <version>_<name>.sql", e.Name())
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("db: migration %q has a non-numeric version: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(fsys, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("db: read %q: %w", e.Name(), err)
		}
		out = append(out, Migration{Version: version, Name: e.Name(), SQL: string(body)})
	}
	seen := make(map[int64]string, len(out))
	for _, m := range out {
		if prev, dup := seen[m.Version]; dup {
			return nil, fmt.Errorf("db: duplicate migration version %d (%s and %s)", m.Version, prev, m.Name)
		}
		seen[m.Version] = m.Name
	}
	// With unique versions this comparator is a total order, so `<` and `<=`
	// behave identically here (the corresponding mutant is equivalent).
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Migrate applies every pending migration. It is idempotent: applied versions are
// recorded in schema_migration and skipped on later runs.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if _, err := pool.Exec(ctx, migrationTable); err != nil {
		return 0, fmt.Errorf("db: create schema_migration: %w", err)
	}
	migrations, err := Migrations()
	if err != nil {
		return 0, err
	}
	applied := 0
	for _, m := range migrations {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM schema_migration WHERE version = $1)", m.Version).Scan(&exists); err != nil {
			return applied, fmt.Errorf("db: check version %d: %w", m.Version, err)
		}
		if exists {
			continue
		}
		// No arguments -> simple protocol -> the file may hold several statements.
		if _, err := pool.Exec(ctx, m.SQL); err != nil {
			return applied, fmt.Errorf("db: apply %s: %w", m.Name, err)
		}
		if _, err := pool.Exec(ctx,
			"INSERT INTO schema_migration (version) VALUES ($1)", m.Version); err != nil {
			return applied, fmt.Errorf("db: record %s: %w", m.Name, err)
		}
		applied++
	}
	return applied, nil
}
