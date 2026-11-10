package klr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// runReference names the records a run wrote, so the report can show where a distributed cost
// came from. The format lives here, next to the only two places that use it.
func runReference(runID int64) string { return fmt.Sprintf("UML:%d", runID) }

// CreateAllocation stores a distribution key.
func (r *Repo) CreateAllocation(ctx context.Context, a Allocation) (Allocation, error) {
	saved := a
	err := r.pool.QueryRow(ctx,
		`INSERT INTO allocation (source_center_id, target_center_id, share_bp, valid_from, valid_to, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		a.SourceCenterID, a.TargetCenterID, a.ShareBP, a.ValidFrom, a.ValidTo, a.CreatedBy).Scan(&saved.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
			return Allocation{}, fmt.Errorf("klr: allocation target: %w", ErrCostCenterNotFound)
		}
		return Allocation{}, fmt.Errorf("klr: create allocation: %w", err)
	}
	return saved, nil
}

// Allocations lists a cost centre's keys, oldest first, so a run always splits in the same
// order and its arithmetic is reproducible.
func (r *Repo) Allocations(ctx context.Context, sourceCenterID int64) ([]Allocation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, source_center_id, target_center_id, share_bp, valid_from, valid_to, created_by
		   FROM allocation WHERE source_center_id = $1 ORDER BY id`, sourceCenterID)
	if err != nil {
		return nil, fmt.Errorf("klr: list allocations: %w", err)
	}
	defer rows.Close()
	out := []Allocation{}
	for rows.Next() {
		var a Allocation
		if err := rows.Scan(&a.ID, &a.SourceCenterID, &a.TargetCenterID, &a.ShareBP,
			&a.ValidFrom, &a.ValidTo, &a.CreatedBy); err != nil {
			return nil, fmt.Errorf("klr: scan allocation: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list allocations: %w", err)
	}
	return out, nil
}

// AllocationRules lists every allocation rule, ordered by source centre and id, so a print view
// reads them the way the Umlage screen shows one centre's.
func (r *Repo) AllocationRules(ctx context.Context) ([]Allocation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, source_center_id, target_center_id, share_bp, valid_from, valid_to, created_by
		   FROM allocation ORDER BY source_center_id, id`)
	if err != nil {
		return nil, fmt.Errorf("klr: list allocation rules: %w", err)
	}
	defer rows.Close()
	out := []Allocation{}
	for rows.Next() {
		var a Allocation
		if err := rows.Scan(&a.ID, &a.SourceCenterID, &a.TargetCenterID, &a.ShareBP,
			&a.ValidFrom, &a.ValidTo, &a.CreatedBy); err != nil {
			return nil, fmt.Errorf("klr: scan allocation rule: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list allocation rules: %w", err)
	}
	return out, nil
}

// CostsInPeriod sums a cost centre's costs booked in the period, counting the closing day in
// full. Revenues are not distributed: they are the Leistungsseite, not a cost to pass on.
func (r *Repo) CostsInPeriod(ctx context.Context, costCenterID int64, from, to time.Time) (int64, error) {
	var cents int64
	err := r.pool.QueryRow(ctx,
		`SELECT coalesce(sum(amount_cents), 0) FROM accounting_record
		  WHERE cost_center_id = $1 AND kind = 'cost'
		    AND business_date >= $2 AND business_date <= $3`,
		costCenterID, from, to).Scan(&cents)
	if err != nil {
		return 0, fmt.Errorf("klr: costs in period: %w", err)
	}
	return cents, nil
}

// RecordRun books a run and the records it produced in one transaction, so a run is never half
// written. A second run over the same cost centre and an overlapping period is refused by the
// table's exclusion constraint, which is the only place that can decide it atomically.
func (r *Repo) RecordRun(ctx context.Context, run Run, records []Record) (Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("klr: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO allocation_run (source_center_id, period_from, period_to, allocated_cents, receivers, run_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		run.SourceCenterID, run.PeriodFrom, run.PeriodTo, run.AllocatedCents, run.Receivers, run.RunBy).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == exclusionViolation {
			return Run{}, fmt.Errorf("klr: run for centre %d over %s..%s: %w",
				run.SourceCenterID, run.PeriodFrom.Format(time.DateOnly),
				run.PeriodTo.Format(time.DateOnly), ErrAlreadyAllocated)
		}
		return Run{}, fmt.Errorf("klr: record run: %w", err)
	}

	reference := runReference(id)
	for _, rec := range records {
		if _, err := tx.Exec(ctx,
			`INSERT INTO accounting_record (cost_center_id, cost_unit_id, kind, amount_cents, business_date, reference, booked_by)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			rec.CostCenterID, rec.CostUnitID, rec.Kind, rec.AmountCents, rec.BusinessDate,
			reference, rec.BookedBy); err != nil {
			return Run{}, fmt.Errorf("klr: record allocation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("klr: commit: %w", err)
	}
	run.ID = id
	return run, nil
}
