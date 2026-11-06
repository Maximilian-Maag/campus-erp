package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TaxRepository is the persistence port for VAT types, tax offices and foundations.
type TaxRepository interface {
	CreateVATType(ctx context.Context, v VATType) (VATType, error)
	UpsertVATType(ctx context.Context, v VATType) (VATType, error)
	GetVATType(ctx context.Context, code string) (VATType, error)
	CreateTaxOffice(ctx context.Context, t TaxOffice) (TaxOffice, error)
	UpsertTaxOffice(ctx context.Context, t TaxOffice) (TaxOffice, error)
	CreateFoundation(ctx context.Context, f Foundation) (Foundation, error)
	UpsertFoundation(ctx context.Context, f Foundation) (Foundation, error)
	Foundations(ctx context.Context, year int) ([]Foundation, error)
}

// TaxRepo is the PostgreSQL implementation.
type TaxRepo struct{ pool *pgxpool.Pool }

// NewTaxRepo returns a TaxRepo.
func NewTaxRepo(pool *pgxpool.Pool) *TaxRepo { return &TaxRepo{pool: pool} }

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// isForeignKeyViolation reports whether err is a PostgreSQL foreign-key error.
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

// CreateVATType stores a VAT type.
func (r *TaxRepo) CreateVATType(ctx context.Context, v VATType) (VATType, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO vat_type (code, description, rate_basis_points, kind)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, code, description, rate_basis_points, kind`,
		v.Code, v.Description, v.RateBasisPoints, v.Kind)
	var out VATType
	if err := row.Scan(&out.ID, &out.Code, &out.Description, &out.RateBasisPoints, &out.Kind); err != nil {
		if isUniqueViolation(err) {
			return VATType{}, fmt.Errorf("kasse: VAT code %q: %w", v.Code, ErrVATCodeExists)
		}
		return VATType{}, fmt.Errorf("kasse: create VAT type: %w", err)
	}
	return out, nil
}

// UpsertVATType inserts a VAT type or updates it in place when the code exists,
// so an import is idempotent (ETL re-runs converge on the same catalogue).
func (r *TaxRepo) UpsertVATType(ctx context.Context, v VATType) (VATType, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO vat_type (code, description, rate_basis_points, kind)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (code) DO UPDATE SET
		   description       = EXCLUDED.description,
		   rate_basis_points = EXCLUDED.rate_basis_points,
		   kind              = EXCLUDED.kind
		 RETURNING id, code, description, rate_basis_points, kind`,
		v.Code, v.Description, v.RateBasisPoints, v.Kind)
	var out VATType
	if err := row.Scan(&out.ID, &out.Code, &out.Description, &out.RateBasisPoints, &out.Kind); err != nil {
		return VATType{}, fmt.Errorf("kasse: upsert VAT type: %w", err)
	}
	return out, nil
}

// GetVATType loads a VAT type by code.
func (r *TaxRepo) GetVATType(ctx context.Context, code string) (VATType, error) {
	var out VATType
	err := r.pool.QueryRow(ctx,
		`SELECT id, code, description, rate_basis_points, kind FROM vat_type WHERE code = $1`, code).
		Scan(&out.ID, &out.Code, &out.Description, &out.RateBasisPoints, &out.Kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return VATType{}, fmt.Errorf("kasse: VAT type %q: %w", code, ErrNotFound)
	}
	if err != nil {
		return VATType{}, fmt.Errorf("kasse: get VAT type: %w", err)
	}
	return out, nil
}

// CreateTaxOffice stores a tax-office master record.
func (r *TaxRepo) CreateTaxOffice(ctx context.Context, t TaxOffice) (TaxOffice, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO tax_office
		   (year, number, name, street, postal_code, city, account_holder, iban, bic)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id, year, number, name, street, postal_code, city, account_holder, iban, bic`,
		t.Year, t.Number, t.Name, t.Street, t.PostalCode, t.City, t.AccountHolder, t.IBAN, t.BIC)
	var out TaxOffice
	if err := row.Scan(&out.ID, &out.Year, &out.Number, &out.Name, &out.Street, &out.PostalCode,
		&out.City, &out.AccountHolder, &out.IBAN, &out.BIC); err != nil {
		if isUniqueViolation(err) {
			return TaxOffice{}, fmt.Errorf("kasse: tax office %d/%s: %w", t.Year, t.Number, ErrTaxOfficeExists)
		}
		return TaxOffice{}, fmt.Errorf("kasse: create tax office: %w", err)
	}
	return out, nil
}

// UpsertTaxOffice inserts a tax-office record or updates it in place when the
// (year, number) pair exists, so an import is idempotent.
func (r *TaxRepo) UpsertTaxOffice(ctx context.Context, t TaxOffice) (TaxOffice, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO tax_office
		   (year, number, name, street, postal_code, city, account_holder, iban, bic)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (year, number) DO UPDATE SET
		   name = EXCLUDED.name, street = EXCLUDED.street,
		   postal_code = EXCLUDED.postal_code, city = EXCLUDED.city,
		   account_holder = EXCLUDED.account_holder,
		   iban = EXCLUDED.iban, bic = EXCLUDED.bic
		 RETURNING id, year, number, name, street, postal_code, city, account_holder, iban, bic`,
		t.Year, t.Number, t.Name, t.Street, t.PostalCode, t.City, t.AccountHolder, t.IBAN, t.BIC)
	var out TaxOffice
	if err := row.Scan(&out.ID, &out.Year, &out.Number, &out.Name, &out.Street, &out.PostalCode,
		&out.City, &out.AccountHolder, &out.IBAN, &out.BIC); err != nil {
		return TaxOffice{}, fmt.Errorf("kasse: upsert tax office: %w", err)
	}
	return out, nil
}

// CreateFoundation stores a foundation reference.
func (r *TaxRepo) CreateFoundation(ctx context.Context, f Foundation) (Foundation, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO foundation (year, number, description, carry_over)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id, year, number, description, carry_over`,
		f.Year, f.Number, f.Description, f.CarryOver)
	var out Foundation
	if err := row.Scan(&out.ID, &out.Year, &out.Number, &out.Description, &out.CarryOver); err != nil {
		if isUniqueViolation(err) {
			return Foundation{}, fmt.Errorf("kasse: foundation %d/%s: %w", f.Year, f.Number, ErrFoundationExists)
		}
		return Foundation{}, fmt.Errorf("kasse: create foundation: %w", err)
	}
	return out, nil
}

// UpsertFoundation inserts a foundation or updates it in place when the
// (year, number) pair exists, so an import is idempotent.
func (r *TaxRepo) UpsertFoundation(ctx context.Context, f Foundation) (Foundation, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO foundation (year, number, description, carry_over)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (year, number) DO UPDATE SET
		   description = EXCLUDED.description, carry_over = EXCLUDED.carry_over
		 RETURNING id, year, number, description, carry_over`,
		f.Year, f.Number, f.Description, f.CarryOver)
	var out Foundation
	if err := row.Scan(&out.ID, &out.Year, &out.Number, &out.Description, &out.CarryOver); err != nil {
		return Foundation{}, fmt.Errorf("kasse: upsert foundation: %w", err)
	}
	return out, nil
}

// Foundations lists foundation references, newest year first. A year of zero lists every
// year, so a caller that has not chosen one still sees what exists.
func (r *TaxRepo) Foundations(ctx context.Context, year int) ([]Foundation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, year, number, description, carry_over
		   FROM foundation
		  WHERE ($1 = 0 OR year = $1)
		  ORDER BY year DESC, number`, year)
	if err != nil {
		return nil, fmt.Errorf("kasse: list foundations: %w", err)
	}
	defer rows.Close()
	out := []Foundation{}
	for rows.Next() {
		var f Foundation
		if err := rows.Scan(&f.ID, &f.Year, &f.Number, &f.Description, &f.CarryOver); err != nil {
			return nil, fmt.Errorf("kasse: scan foundation: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
