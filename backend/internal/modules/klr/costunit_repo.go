package klr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// costUnitRunReference names the records a Kostenträger run wrote, so the report can show where
// an attributed cost came from. It is a different prefix from the centre-side run's, so a
// reference always says which engine wrote it.
func costUnitRunReference(runID int64) string { return fmt.Sprintf("KTU:%d", runID) }

// GetCostUnit loads a cost unit by id.
func (r *Repo) GetCostUnit(ctx context.Context, id int64) (CostUnit, error) {
	var u CostUnit
	err := r.pool.QueryRow(ctx,
		`SELECT id, number, name, created_by FROM cost_unit WHERE id = $1`, id).
		Scan(&u.ID, &u.Number, &u.Name, &u.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return CostUnit{}, fmt.Errorf("klr: cost unit %d: %w", id, ErrCostUnitNotFound)
	}
	if err != nil {
		return CostUnit{}, fmt.Errorf("klr: get cost unit: %w", err)
	}
	return u, nil
}

// CreateCostUnitKey stores a Kostenträger key.
func (r *Repo) CreateCostUnitKey(ctx context.Context, k CostUnitKey) (CostUnitKey, error) {
	saved := k
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cost_unit_key (source_center_id, target_unit_id, share_bp, valid_from, valid_to, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		k.SourceCenterID, k.TargetUnitID, k.ShareBP, k.ValidFrom, k.ValidTo, k.CreatedBy).Scan(&saved.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
			return CostUnitKey{}, fmt.Errorf("klr: cost unit key target: %w", ErrCostUnitNotFound)
		}
		return CostUnitKey{}, fmt.Errorf("klr: create cost unit key: %w", err)
	}
	return saved, nil
}

// CostUnitKeys lists a cost centre's keys, oldest first, so a run always splits in the same order
// and its arithmetic is reproducible.
func (r *Repo) CostUnitKeys(ctx context.Context, sourceCenterID int64) ([]CostUnitKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, source_center_id, target_unit_id, share_bp, valid_from, valid_to, created_by
		   FROM cost_unit_key WHERE source_center_id = $1 ORDER BY id`, sourceCenterID)
	if err != nil {
		return nil, fmt.Errorf("klr: list cost unit keys: %w", err)
	}
	defer rows.Close()
	out := []CostUnitKey{}
	for rows.Next() {
		var k CostUnitKey
		if err := rows.Scan(&k.ID, &k.SourceCenterID, &k.TargetUnitID, &k.ShareBP,
			&k.ValidFrom, &k.ValidTo, &k.CreatedBy); err != nil {
			return nil, fmt.Errorf("klr: scan cost unit key: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list cost unit keys: %w", err)
	}
	return out, nil
}

// RecordCostUnitRun books a Kostenträger run and the records it produced in one transaction, so a
// run is never half written. A second run over the same cost centre and an overlapping period is
// refused by the table's exclusion constraint, which is the only place that can decide it
// atomically.
func (r *Repo) RecordCostUnitRun(ctx context.Context, run Run, records []Record) (Run, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("klr: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO cost_unit_run (source_center_id, period_from, period_to, allocated_cents, receivers, run_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		run.SourceCenterID, run.PeriodFrom, run.PeriodTo, run.AllocatedCents, run.Receivers, run.RunBy).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == exclusionViolation {
			return Run{}, fmt.Errorf("klr: cost unit run for centre %d over %s..%s: %w",
				run.SourceCenterID, run.PeriodFrom.Format(time.DateOnly),
				run.PeriodTo.Format(time.DateOnly), ErrAlreadyAllocated)
		}
		return Run{}, fmt.Errorf("klr: record cost unit run: %w", err)
	}

	reference := costUnitRunReference(id)
	for _, rec := range records {
		if _, err := tx.Exec(ctx,
			`INSERT INTO accounting_record (cost_center_id, cost_unit_id, kind, amount_cents, business_date, reference, booked_by)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			rec.CostCenterID, rec.CostUnitID, rec.Kind, rec.AmountCents, rec.BusinessDate,
			reference, rec.BookedBy); err != nil {
			return Run{}, fmt.Errorf("klr: record cost unit allocation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("klr: commit: %w", err)
	}
	run.ID = id
	return run, nil
}
