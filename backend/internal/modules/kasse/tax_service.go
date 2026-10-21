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

// VATType loads a VAT type by code.
func (s *TaxService) VATType(ctx context.Context, code string) (VATType, error) {
	return s.repo.GetVATType(ctx, code)
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
