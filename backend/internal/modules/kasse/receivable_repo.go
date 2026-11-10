package kasse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReceivableRepository is the persistence port for open receivables.
type ReceivableRepository interface {
	CreateReceivable(ctx context.Context, r Receivable, by string) (Receivable, error)
	GetReceivable(ctx context.Context, id int64) (Receivable, error)
	Receivables(ctx context.Context, state ReceivableState) ([]Receivable, error)
	OpenReceivablesForCollection(ctx context.Context, dueBy time.Time, sequenceType MandateSequence) ([]Receivable, error)
}

// ReceivableRepo is the PostgreSQL implementation.
type ReceivableRepo struct{ pool *pgxpool.Pool }

// NewReceivableRepo returns a ReceivableRepo.
func NewReceivableRepo(pool *pgxpool.Pool) *ReceivableRepo { return &ReceivableRepo{pool: pool} }

const receivableColumns = `id, mandate_reference, amount_cents, currency, due_date, purpose,
	state, collected_run_id, source, created_by`

// receivableColumnsQualified is the same list prefixed with the receivable alias,
// for the join where the mandate's own id and reference would be ambiguous.
const receivableColumnsQualified = `r.id, r.mandate_reference, r.amount_cents, r.currency, r.due_date,
	r.purpose, r.state, r.collected_run_id, r.source, r.created_by`

func scanReceivable(row pgx.Row) (Receivable, error) {
	var r Receivable
	err := row.Scan(&r.ID, &r.MandateReference, &r.AmountCents, &r.Currency, &r.DueDate,
		&r.Purpose, &r.State, &r.CollectedRunID, &r.Source, &r.CreatedBy)
	return r, err
}

// CreateReceivable stores an open item. A mandate that does not exist maps to
// ErrMandateNotFound through the foreign key.
func (r *ReceivableRepo) CreateReceivable(ctx context.Context, rec Receivable, by string) (Receivable, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO receivable
		   (mandate_reference, amount_cents, currency, due_date, purpose, state, source, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING `+receivableColumns,
		rec.MandateReference, rec.AmountCents, rec.Currency, rec.DueDate, rec.Purpose,
		string(ReceivableOpen), rec.Source, by)
	out, err := scanReceivable(row)
	if err != nil {
		if isForeignKeyViolation(err) {
			return Receivable{}, fmt.Errorf("kasse: receivable mandate %q: %w", rec.MandateReference, ErrMandateNotFound)
		}
		return Receivable{}, fmt.Errorf("kasse: create receivable: %w", err)
	}
	return out, nil
}

// GetReceivable loads a receivable by id.
func (r *ReceivableRepo) GetReceivable(ctx context.Context, id int64) (Receivable, error) {
	out, err := scanReceivable(r.pool.QueryRow(ctx,
		`SELECT `+receivableColumns+` FROM receivable WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Receivable{}, fmt.Errorf("kasse: receivable %d: %w", id, ErrReceivableNotFound)
	}
	if err != nil {
		return Receivable{}, fmt.Errorf("kasse: get receivable: %w", err)
	}
	return out, nil
}

// Receivables lists open items, due date first — the order an operator chases
// them in — optionally narrowed to one state (the empty state lists all).
func (r *ReceivableRepo) Receivables(ctx context.Context, state ReceivableState) ([]Receivable, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+receivableColumns+` FROM receivable
		  WHERE ($1 = '' OR state = $1)
		  ORDER BY due_date, id`, string(state))
	if err != nil {
		return nil, fmt.Errorf("kasse: list receivables: %w", err)
	}
	defer rows.Close()
	out := []Receivable{}
	for rows.Next() {
		rec, err := scanReceivable(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan receivable: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: list receivables: %w", err)
	}
	return out, nil
}

// OpenReceivablesDueBy selects the open items a dunning run walks: still open and
// due on or before asOf. It does not require an active mandate or a sequence type —
// dunning happens when collection did not — so the debt is chased whatever its
// collection status. Due date first, the order an operator chases them in.
func (r *ReceivableRepo) OpenReceivablesDueBy(ctx context.Context, asOf time.Time) ([]Receivable, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+receivableColumns+` FROM receivable
		  WHERE state = 'open' AND due_date <= $1
		  ORDER BY due_date, id`, asOf)
	if err != nil {
		return nil, fmt.Errorf("kasse: select receivables for dunning: %w", err)
	}
	defer rows.Close()
	out := []Receivable{}
	for rows.Next() {
		rec, err := scanReceivable(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan receivable: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: select receivables for dunning: %w", err)
	}
	return out, nil
}

// OpenReceivablesForCollection selects the open items a run can gather: due on
// or before dueBy, under an active mandate whose sequence type is the run's. A
// receivable whose mandate is inactive or whose sequence type differs is not
// selectable, so it is not returned — the run cannot collect it.
func (r *ReceivableRepo) OpenReceivablesForCollection(ctx context.Context, dueBy time.Time, sequenceType MandateSequence) ([]Receivable, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+receivableColumnsQualified+`
		   FROM receivable r
		   JOIN direct_debit_mandate m ON m.mandate_reference = r.mandate_reference
		  WHERE r.state = 'open'
		    AND r.due_date <= $1
		    AND m.active
		    AND m.sequence_type = $2
		  ORDER BY r.mandate_reference, r.due_date, r.id`, dueBy, string(sequenceType))
	if err != nil {
		return nil, fmt.Errorf("kasse: select receivables for collection: %w", err)
	}
	defer rows.Close()
	out := []Receivable{}
	for rows.Next() {
		rec, err := scanReceivable(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan receivable: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: select receivables for collection: %w", err)
	}
	return out, nil
}
