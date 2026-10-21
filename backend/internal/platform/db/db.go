// Package db opens tenant-scoped PostgreSQL pools and applies migrations.
//
// One database per tenant (ADR-0002): the DSN points at a single tenant's
// database; there is no cross-tenant query path.
package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvDatabaseURL is the environment variable holding the tenant DSN.
const EnvDatabaseURL = "DATABASE_URL"

// Open returns a connection pool for the given DSN.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("db: empty DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse DSN: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// DSNFromEnv reads the tenant DSN from the environment.
func DSNFromEnv() (string, error) {
	dsn := os.Getenv(EnvDatabaseURL)
	if dsn == "" {
		return "", fmt.Errorf("db: %s is required", EnvDatabaseURL)
	}
	return dsn, nil
}
