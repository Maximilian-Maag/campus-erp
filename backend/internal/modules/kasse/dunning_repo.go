package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DunningRepository is the persistence port for dunning configuration and notices.
type DunningRepository interface {
	CreateLevel(ctx context.Context, l DunningLevel) (DunningLevel, error)
	GetLevel(ctx context.Context, year int, code string) (DunningLevel, error)
	InsertNotice(ctx context.Context, n DunningNotice) (DunningNotice, error)
}

// DunningRepo is the PostgreSQL implementation.
type DunningRepo struct{ pool *pgxpool.Pool }

// NewDunningRepo returns a DunningRepo.
func NewDunningRepo(pool *pgxpool.Pool) *DunningRepo { return &DunningRepo{pool: pool} }

// CreateLevel stores a level and its bands in one transaction.
func (r *DunningRepo) CreateLevel(ctx context.Context, l DunningLevel) (DunningLevel, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return DunningLevel{}, fmt.Errorf("kasse: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO dunning_level (year, code, description, days_allowed, active, next_code)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		l.Year, l.Code, l.Description, l.DaysAllowed, l.Active, l.NextCode).Scan(&id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return DunningLevel{}, fmt.Errorf("kasse: dunning level %d/%s: %w", l.Year, l.Code, ErrLevelExists)
		}
		return DunningLevel{}, fmt.Errorf("kasse: create dunning level: %w", err)
	}
	for i, b := range l.Bands {
		if _, err := tx.Exec(ctx,
			`INSERT INTO dunning_band (level_id, position, up_to_cents, fee_cents, rate_basis_points)
			 VALUES ($1,$2,$3,$4,$5)`,
			id, i+1, b.UpToCents, b.FeeCents, b.RateBasisPoints); err != nil {
			return DunningLevel{}, fmt.Errorf("kasse: create dunning band %d: %w", i+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DunningLevel{}, fmt.Errorf("kasse: commit: %w", err)
	}
	l.ID = id
	return l, nil
}

// GetLevel loads a level and its bands, ordered by position.
func (r *DunningRepo) GetLevel(ctx context.Context, year int, code string) (DunningLevel, error) {
	var l DunningLevel
	err := r.pool.QueryRow(ctx,
		`SELECT id, year, code, description, days_allowed, active, next_code
		   FROM dunning_level WHERE year = $1 AND code = $2`, year, code).
		Scan(&l.ID, &l.Year, &l.Code, &l.Description, &l.DaysAllowed, &l.Active, &l.NextCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return DunningLevel{}, fmt.Errorf("kasse: dunning level %d/%s: %w", year, code, ErrNotFound)
	}
	if err != nil {
		return DunningLevel{}, fmt.Errorf("kasse: get dunning level: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT up_to_cents, fee_cents, rate_basis_points
		   FROM dunning_band WHERE level_id = $1 ORDER BY position`, l.ID)
	if err != nil {
		return DunningLevel{}, fmt.Errorf("kasse: get dunning bands: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b DunningBand
		if err := rows.Scan(&b.UpToCents, &b.FeeCents, &b.RateBasisPoints); err != nil {
			return DunningLevel{}, fmt.Errorf("kasse: scan dunning band: %w", err)
		}
		l.Bands = append(l.Bands, b)
	}
	return l, rows.Err()
}

// InsertNotice stores an issued notice.
func (r *DunningRepo) InsertNotice(ctx context.Context, n DunningNotice) (DunningNotice, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO dunning_notice
		   (claim_reference, level_code, outstanding_cents, charge_cents, issued_on, issued_by)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id`,
		n.ClaimReference, n.LevelCode, n.OutstandingCents, n.ChargeCents, n.IssuedOn, n.IssuedBy)
	if err := row.Scan(&n.ID); err != nil {
		return DunningNotice{}, fmt.Errorf("kasse: insert dunning notice: %w", err)
	}
	return n, nil
}
