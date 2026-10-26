package klr

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"
const foreignKeyViolation = "23503"

// Repository is the persistence port for the KLR.
type Repository interface {
	CreateCostCenter(ctx context.Context, c CostCenter) (CostCenter, error)
	GetCostCenter(ctx context.Context, id int64) (CostCenter, error)
	CreateCostUnit(ctx context.Context, u CostUnit) (CostUnit, error)
	InsertRecord(ctx context.Context, r Record) (Record, error)
	Report(ctx context.Context, costCenterID int64) (Report, error)
}

// Repo is the PostgreSQL implementation.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo returns a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// CreateCostCenter stores a cost centre.
func (r *Repo) CreateCostCenter(ctx context.Context, c CostCenter) (CostCenter, error) {
	saved := c
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cost_center (number, name, created_by) VALUES ($1,$2,$3)
		 RETURNING id`, c.Number, c.Name, c.CreatedBy).Scan(&saved.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return CostCenter{}, ErrCostCenterExists
		}
		return CostCenter{}, fmt.Errorf("klr: create cost centre: %w", err)
	}
	return saved, nil
}

// GetCostCenter loads a cost centre.
func (r *Repo) GetCostCenter(ctx context.Context, id int64) (CostCenter, error) {
	var c CostCenter
	err := r.pool.QueryRow(ctx,
		`SELECT id, number, name, created_by FROM cost_center WHERE id = $1`, id).
		Scan(&c.ID, &c.Number, &c.Name, &c.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return CostCenter{}, fmt.Errorf("klr: cost centre %d: %w", id, ErrCostCenterNotFound)
	}
	if err != nil {
		return CostCenter{}, fmt.Errorf("klr: get cost centre: %w", err)
	}
	return c, nil
}

// CreateCostUnit stores a cost unit.
func (r *Repo) CreateCostUnit(ctx context.Context, u CostUnit) (CostUnit, error) {
	saved := u
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cost_unit (number, name, created_by) VALUES ($1,$2,$3)
		 RETURNING id`, u.Number, u.Name, u.CreatedBy).Scan(&saved.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return CostUnit{}, ErrCostUnitExists
		}
		return CostUnit{}, fmt.Errorf("klr: create cost unit: %w", err)
	}
	return saved, nil
}

// InsertRecord appends a KLR posting.
func (r *Repo) InsertRecord(ctx context.Context, rec Record) (Record, error) {
	saved := rec
	err := r.pool.QueryRow(ctx,
		`INSERT INTO accounting_record (cost_center_id, cost_unit_id, kind, amount_cents, reference, booked_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		rec.CostCenterID, rec.CostUnitID, rec.Kind, rec.AmountCents, rec.Reference, rec.BookedBy).Scan(&saved.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
			return Record{}, fmt.Errorf("klr: cost centre %d: %w", rec.CostCenterID, ErrCostCenterNotFound)
		}
		return Record{}, fmt.Errorf("klr: insert record: %w", err)
	}
	return saved, nil
}

// Report sums a cost centre's costs and revenues.
func (r *Repo) Report(ctx context.Context, costCenterID int64) (Report, error) {
	if _, err := r.GetCostCenter(ctx, costCenterID); err != nil {
		return Report{}, err
	}
	rep := Report{CostCenterID: costCenterID}
	err := r.pool.QueryRow(ctx,
		`SELECT coalesce(sum(amount_cents) FILTER (WHERE kind = 'cost'), 0),
		        coalesce(sum(amount_cents) FILTER (WHERE kind = 'revenue'), 0)
		   FROM accounting_record WHERE cost_center_id = $1`, costCenterID).
		Scan(&rep.CostsCents, &rep.RevenuesCents)
	if err != nil {
		return Report{}, fmt.Errorf("klr: report: %w", err)
	}
	return rep, nil
}
