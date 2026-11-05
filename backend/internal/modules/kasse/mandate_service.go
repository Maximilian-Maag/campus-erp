package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// MandateService implements the SEPA mandate use cases.
type MandateService struct {
	repo  MandateRepository
	audit AuditSink
}

// NewMandateService wires the mandate service.
func NewMandateService(repo MandateRepository, sink AuditSink) *MandateService {
	return &MandateService{repo: repo, audit: sink}
}

// CreateMandate stores a mandate.
func (s *MandateService) CreateMandate(ctx context.Context, m Mandate, by string) (Mandate, error) {
	if err := requireActor(by); err != nil {
		return Mandate{}, err
	}
	saved, err := s.repo.CreateMandate(ctx, m, by)
	if err != nil {
		return Mandate{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "mandate.create",
		Subject: "mandate/" + saved.Reference,
		After:   fmt.Sprintf("sequence_type=%s debtor_iban=%s active=%t", saved.SequenceType, saved.DebtorIBAN, saved.Active),
	}); err != nil {
		return Mandate{}, err
	}
	return saved, nil
}

// Mandate loads a mandate by reference.
func (s *MandateService) Mandate(ctx context.Context, reference string) (Mandate, error) {
	return s.repo.GetMandate(ctx, reference)
}

// Mandates lists the mandates, optionally only the active ones and optionally only
// those collecting under one sequence type — what a screen needs to offer the
// mandates a run can actually gather.
func (s *MandateService) Mandates(ctx context.Context, activeOnly bool, seq MandateSequence) ([]Mandate, error) {
	return s.repo.Mandates(ctx, activeOnly, seq)
}
