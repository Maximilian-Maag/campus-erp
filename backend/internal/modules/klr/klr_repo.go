package klr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"

// exclusionViolation is PostgreSQL's SQLSTATE for an exclusion-constraint violation, which is
// what the allocation run's period overlap is declared as.
const exclusionViolation = "23P01"
const foreignKeyViolation = "23503"

// Repository is the persistence port for the KLR.
type Repository interface {
	CreateCostCenter(ctx context.Context, c CostCenter) (CostCenter, error)
	GetCostCenter(ctx context.Context, id int64) (CostCenter, error)
	CostCenters(ctx context.Context) ([]CostCenter, error)
	CreateCostUnit(ctx context.Context, u CostUnit) (CostUnit, error)
	InsertRecord(ctx context.Context, r Record) (Record, error)
	Report(ctx context.Context, costCenterID int64) (Report, error)

	CreateAllocation(ctx context.Context, a Allocation) (Allocation, error)
	Allocations(ctx context.Context, sourceCenterID int64) ([]Allocation, error)
	CostUnitFigures(ctx context.Context) ([]CostUnitLine, error)
	CostsInPeriod(ctx context.Context, costCenterID int64, from, to time.Time) (int64, error)
	RecordRun(ctx context.Context, run Run, records []Record) (Run, error)

	CreateCostType(ctx context.Context, t CostType) (CostType, error)
	GetCostType(ctx context.Context, id int64) (CostType, error)
	CostTypes(ctx context.Context) ([]CostType, error)
	MapAccount(ctx context.Context, link CostTypeAccount) (CostTypeAccount, error)
	AccountsOf(ctx context.Context, costTypeID int64) ([]CostTypeAccount, error)
	CostTypeByAccount(ctx context.Context, accountCode string) (CostType, error)
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

// CostCenters lists every cost centre by number, which is the order an operator reads them in
// and the order a screen shows them.
func (r *Repo) CostCenters(ctx context.Context) ([]CostCenter, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, number, name, created_by FROM cost_center ORDER BY number`)
	if err != nil {
		return nil, fmt.Errorf("klr: list cost centres: %w", err)
	}
	defer rows.Close()
	out := []CostCenter{}
	for rows.Next() {
		var c CostCenter
		if err := rows.Scan(&c.ID, &c.Number, &c.Name, &c.CreatedBy); err != nil {
			return nil, fmt.Errorf("klr: scan cost centre: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list cost centres: %w", err)
	}
	return out, nil
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
		`INSERT INTO accounting_record (cost_center_id, cost_unit_id, cost_type_id, kind, amount_cents, business_date, reference, booked_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		rec.CostCenterID, rec.CostUnitID, rec.CostTypeID, rec.Kind, rec.AmountCents, rec.BusinessDate,
		rec.Reference, rec.BookedBy).Scan(&saved.ID)
	if err != nil {
		// Which foreign key failed says which reference was unknown.
		switch {
		case isForeignKeyViolationOn(err, "accounting_record_cost_center_id_fkey"):
			return Record{}, fmt.Errorf("klr: cost centre %d: %w", rec.CostCenterID, ErrCostCenterNotFound)
		case isForeignKeyViolationOn(err, "accounting_record_cost_unit_id_fkey"):
			return Record{}, fmt.Errorf("klr: cost unit: %w", ErrCostUnitNotFound)
		case isForeignKeyViolationOn(err, "accounting_record_cost_type_id_fkey"):
			return Record{}, fmt.Errorf("klr: cost type: %w", ErrCostTypeNotFound)
		}
		return Record{}, fmt.Errorf("klr: insert record: %w", err)
	}
	return saved, nil
}

// CostUnitFigures lists every cost unit with the costs and revenues booked on it. A unit with no
// records is listed with zeroes rather than left out: "nothing is booked on it yet" is a fact
// about the unit, and a report that hides it reads as if the unit did not exist.
func (r *Repo) CostUnitFigures(ctx context.Context) ([]CostUnitLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.number, u.name,
		        COALESCE(SUM(CASE WHEN a.kind = $1 THEN a.amount_cents ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN a.kind = $2 THEN a.amount_cents ELSE 0 END), 0)
		   FROM cost_unit u
		   LEFT JOIN accounting_record a ON a.cost_unit_id = u.id
		  GROUP BY u.id, u.number, u.name
		  ORDER BY u.number`, Cost, Revenue)
	if err != nil {
		return nil, fmt.Errorf("klr: cost unit figures: %w", err)
	}
	defer rows.Close()
	out := []CostUnitLine{}
	for rows.Next() {
		var line CostUnitLine
		if err := rows.Scan(&line.CostUnit.ID, &line.CostUnit.Number, &line.CostUnit.Name,
			&line.CostsCents, &line.RevenuesCents); err != nil {
			return nil, fmt.Errorf("klr: scan cost unit figure: %w", err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: cost unit figures: %w", err)
	}
	return out, nil
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
