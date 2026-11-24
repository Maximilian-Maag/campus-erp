package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MandateRepository is the persistence port for SEPA mandates.
type MandateRepository interface {
	CreateMandate(ctx context.Context, m Mandate, by string) (Mandate, error)
	GetMandate(ctx context.Context, reference string) (Mandate, error)
	Mandates(ctx context.Context, activeOnly bool, seq MandateSequence, limit, offset int) ([]Mandate, error)
	// RetireMandate withdraws a mandate's authorisation, keeping the row.
	RetireMandate(ctx context.Context, reference string) (Mandate, error)
	// AdvanceSequenceType moves a mandate from one sequence type to another, compare-and-set on the
	// type it is moving from: a mandate another run advanced in the meantime is left alone and the
	// bool reports false, so a decision taken on a stale read never overwrites a newer one.
	AdvanceSequenceType(ctx context.Context, reference string, from, to MandateSequence) (Mandate, bool, error)
}

// MandateRepo is the PostgreSQL implementation.
type MandateRepo struct{ pool *pgxpool.Pool }

// NewMandateRepo returns a MandateRepo.
func NewMandateRepo(pool *pgxpool.Pool) *MandateRepo { return &MandateRepo{pool: pool} }

const mandateColumns = `id, mandate_reference, debtor_name, debtor_iban, debtor_bic, signature_date, sequence_type, active`

func scanMandate(row pgx.Row) (Mandate, error) {
	var m Mandate
	err := row.Scan(&m.ID, &m.Reference, &m.DebtorName, &m.DebtorIBAN, &m.DebtorBIC,
		&m.SignatureDate, &m.SequenceType, &m.Active)
	return m, err
}

// CreateMandate stores a mandate.
func (r *MandateRepo) CreateMandate(ctx context.Context, m Mandate, by string) (Mandate, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO direct_debit_mandate
		   (mandate_reference, debtor_name, debtor_iban, debtor_bic, signature_date, sequence_type, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING `+mandateColumns,
		m.Reference, m.DebtorName, m.DebtorIBAN, m.DebtorBIC, m.SignatureDate, m.SequenceType, by)
	out, err := scanMandate(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Mandate{}, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateReferenceExists)
		}
		return Mandate{}, fmt.Errorf("kasse: create mandate: %w", err)
	}
	return out, nil
}

// GetMandate loads a mandate by reference.
func (r *MandateRepo) GetMandate(ctx context.Context, reference string) (Mandate, error) {
	out, err := scanMandate(r.pool.QueryRow(ctx,
		`SELECT `+mandateColumns+` FROM direct_debit_mandate WHERE mandate_reference = $1`, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return Mandate{}, fmt.Errorf("kasse: mandate %q: %w", reference, ErrMandateNotFound)
	}
	if err != nil {
		return Mandate{}, fmt.Errorf("kasse: get mandate: %w", err)
	}
	return out, nil
}

// RetireMandate clears a mandate's active flag and returns the row as it now stands. The reference
// is the identity, so nothing is deleted: the mandate stays readable, which is what a revocation
// trail needs.
func (r *MandateRepo) RetireMandate(ctx context.Context, reference string) (Mandate, error) {
	out, err := scanMandate(r.pool.QueryRow(ctx,
		`UPDATE direct_debit_mandate SET active = false
		  WHERE mandate_reference = $1
		  RETURNING `+mandateColumns, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return Mandate{}, fmt.Errorf("kasse: mandate %q: %w", reference, ErrMandateNotFound)
	}
	if err != nil {
		return Mandate{}, fmt.Errorf("kasse: retire mandate: %w", err)
	}
	return out, nil
}

// AdvanceSequenceType moves a mandate from one sequence type to another. The type it moves from is
// part of the WHERE clause, so the update is a compare-and-set: two callers holding the same read
// cannot both advance it, and the second one is told (false) rather than writing a type it never
// saw. A reference that does not exist is reported as such, so a caller can tell "gone" from "already
// advanced".
func (r *MandateRepo) AdvanceSequenceType(ctx context.Context, reference string, from, to MandateSequence) (Mandate, bool, error) {
	if !ValidSequenceType(from) || !ValidSequenceType(to) {
		return Mandate{}, false, ErrSequenceTypeUnknown
	}
	out, err := scanMandate(r.pool.QueryRow(ctx,
		`UPDATE direct_debit_mandate SET sequence_type = $2
		  WHERE mandate_reference = $1 AND sequence_type = $3
		  RETURNING `+mandateColumns, reference, string(to), string(from)))
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing moved: either the mandate is gone or it is no longer at the type the caller read.
		// Reading it back tells the two apart, and costs one query only on the path that failed.
		if _, err := r.GetMandate(ctx, reference); err != nil {
			return Mandate{}, false, err
		}
		return Mandate{}, false, nil
	}
	if err != nil {
		return Mandate{}, false, fmt.Errorf("kasse: advance mandate sequence type: %w", err)
	}
	return out, true, nil
}

// Mandates lists the mandates by reference, one page at a time. The filter is the store's, so a
// caller that wants the live ones collecting under one sequence type never holds every mandate to
// show a few: a false activeOnly ignores it, a blank seq ignores it.
//
// Ordered by reference, which is UNIQUE, so the order is total: a page boundary can neither repeat
// nor skip a row, unlike an order over a column two rows can share.
func (r *MandateRepo) Mandates(ctx context.Context, activeOnly bool, seq MandateSequence, limit, offset int) ([]Mandate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+mandateColumns+` FROM direct_debit_mandate
		  WHERE ($1 = false OR active = $1)
		    AND ($2 = '' OR sequence_type = $2)
		  ORDER BY mandate_reference LIMIT $3 OFFSET $4`, activeOnly, string(seq), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("kasse: list mandates: %w", err)
	}
	defer rows.Close()
	out := []Mandate{}
	for rows.Next() {
		m, err := scanMandate(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan mandate: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: list mandates: %w", err)
	}
	return out, nil
}
