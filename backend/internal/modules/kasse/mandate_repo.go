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
	Mandates(ctx context.Context, activeOnly bool, seq MandateSequence) ([]Mandate, error)
	// RetireMandate withdraws a mandate's authorisation, keeping the row.
	RetireMandate(ctx context.Context, reference string) (Mandate, error)
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

// Mandates lists the mandates by reference. The filter is the store's, so a caller
// that wants the live ones collecting under one sequence type never holds every
// mandate to show a few: a false activeOnly ignores it, a blank seq ignores it.
func (r *MandateRepo) Mandates(ctx context.Context, activeOnly bool, seq MandateSequence) ([]Mandate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+mandateColumns+` FROM direct_debit_mandate
		  WHERE ($1 = false OR active = $1)
		    AND ($2 = '' OR sequence_type = $2)
		  ORDER BY mandate_reference`, activeOnly, string(seq))
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
