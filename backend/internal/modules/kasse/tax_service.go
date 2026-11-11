package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// TaxService implements tax-office data and foundations (S7).
type TaxService struct {
	repo  TaxRepository
	audit AuditSink
}

// NewTaxService wires the tax service.
func NewTaxService(repo TaxRepository, sink AuditSink) *TaxService {
	return &TaxService{repo: repo, audit: sink}
}

// CreateVATType stores a VAT type.
func (s *TaxService) CreateVATType(ctx context.Context, v VATType, by string) (VATType, error) {
	if err := requireActor(by); err != nil {
		return VATType{}, err
	}
	saved, err := s.repo.CreateVATType(ctx, v)
	if err != nil {
		return VATType{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "vat_type.create",
		Subject: "vat-type/" + saved.Code,
		After:   fmt.Sprintf("rate_basis_points=%d kind=%s", saved.RateBasisPoints, saved.Kind),
	}); err != nil {
		return VATType{}, err
	}
	return saved, nil
}

// ImportVATTypes validates and upserts a batch (used by the ETL), auditing once.
// It validates every entry before writing any, so a bad row fails the import
// without a partial load.
func (s *TaxService) ImportVATTypes(ctx context.Context, values []VATType, by string) (int, error) {
	if err := requireActor(by); err != nil {
		return 0, err
	}
	clean := make([]VATType, 0, len(values))
	for _, v := range values {
		c, err := NewVATType(v.Code, v.Description, v.RateBasisPoints, v.Kind)
		if err != nil {
			return 0, err
		}
		clean = append(clean, c)
	}
	for _, c := range clean {
		if _, err := s.repo.UpsertVATType(ctx, c); err != nil {
			return 0, err
		}
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "vat_type.import",
		Subject: "vat-type",
		After:   fmt.Sprintf("types=%d", len(clean)),
	}); err != nil {
		return 0, err
	}
	return len(clean), nil
}

// VATType loads a VAT type by code.
func (s *TaxService) VATType(ctx context.Context, code string) (VATType, error) {
	return s.repo.GetVATType(ctx, code)
}

// VATTypes lists the VAT types — the catalogue the exact VAT arithmetic is driven from, so a screen
// can show what a rate means rather than only what it is.
func (s *TaxService) VATTypes(ctx context.Context) ([]VATType, error) {
	return s.repo.VATTypes(ctx)
}

// TaxOffices lists the tax offices, optionally narrowed to one year (0 = every year). The filter is
// the store's: a screen asks for the year it shows instead of holding every year to sift a few.
func (s *TaxService) TaxOffices(ctx context.Context, year int) ([]TaxOffice, error) {
	return s.repo.TaxOffices(ctx, year)
}

// CreateTaxOffice stores a tax-office master record.
func (s *TaxService) CreateTaxOffice(ctx context.Context, t TaxOffice, by string) (TaxOffice, error) {
	if err := requireActor(by); err != nil {
		return TaxOffice{}, err
	}
	saved, err := s.repo.CreateTaxOffice(ctx, t)
	if err != nil {
		return TaxOffice{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "tax_office.create",
		Subject: fmt.Sprintf("tax-office/%d/%s", saved.Year, saved.Number),
		After:   "name=" + saved.Name,
	}); err != nil {
		return TaxOffice{}, err
	}
	return saved, nil
}

// ImportTaxOffices validates and upserts a batch (used by the ETL), auditing once.
func (s *TaxService) ImportTaxOffices(ctx context.Context, values []TaxOffice, by string) (int, error) {
	if err := requireActor(by); err != nil {
		return 0, err
	}
	clean := make([]TaxOffice, 0, len(values))
	for _, t := range values {
		c, err := NewTaxOffice(t.Year, t.Number, t.Name, t.Street, t.PostalCode, t.City, t.AccountHolder, t.IBAN, t.BIC)
		if err != nil {
			return 0, err
		}
		clean = append(clean, c)
	}
	for _, c := range clean {
		if _, err := s.repo.UpsertTaxOffice(ctx, c); err != nil {
			return 0, err
		}
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "tax_office.import",
		Subject: "tax-office",
		After:   fmt.Sprintf("offices=%d", len(clean)),
	}); err != nil {
		return 0, err
	}
	return len(clean), nil
}

// ImportFoundations validates and upserts a batch (used by the ETL), auditing once.
func (s *TaxService) ImportFoundations(ctx context.Context, values []Foundation, by string) (int, error) {
	if err := requireActor(by); err != nil {
		return 0, err
	}
	clean := make([]Foundation, 0, len(values))
	for _, f := range values {
		c, err := NewFoundation(f.Year, f.Number, f.Description, f.CarryOver)
		if err != nil {
			return 0, err
		}
		clean = append(clean, c)
	}
	for _, c := range clean {
		if _, err := s.repo.UpsertFoundation(ctx, c); err != nil {
			return 0, err
		}
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "foundation.import",
		Subject: "foundation",
		After:   fmt.Sprintf("foundations=%d", len(clean)),
	}); err != nil {
		return 0, err
	}
	return len(clean), nil
}

// CreateFoundation stores a foundation reference.
func (s *TaxService) CreateFoundation(ctx context.Context, f Foundation, by string) (Foundation, error) {
	if err := requireActor(by); err != nil {
		return Foundation{}, err
	}
	saved, err := s.repo.CreateFoundation(ctx, f)
	if err != nil {
		return Foundation{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "foundation.create",
		Subject: fmt.Sprintf("foundation/%d/%s", saved.Year, saved.Number),
		After:   fmt.Sprintf("description=%s carry_over=%t", saved.Description, saved.CarryOver),
	}); err != nil {
		return Foundation{}, err
	}
	return saved, nil
}

// Foundations lists foundation references, optionally narrowed to one year. Reading is the
// server's: a year of zero lists every year.
func (s *TaxService) Foundations(ctx context.Context, year int) ([]Foundation, error) {
	return s.repo.Foundations(ctx, year)
}
