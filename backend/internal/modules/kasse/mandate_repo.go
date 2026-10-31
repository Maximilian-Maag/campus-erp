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
