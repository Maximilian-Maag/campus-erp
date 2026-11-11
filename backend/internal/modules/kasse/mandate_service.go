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

// RetireMandate withdraws a mandate's authorisation (docs/21). The mandate is read first, so a
// reference nothing carries is a not-found *before* the state is asked about, and a mandate that is
// already retired is refused (`ErrMandateRetired`) rather than quietly left as it is. The write is
// audited with the state it came from, so a revocation is readable in the trail and not only in the
// flag it left behind.
func (s *MandateService) RetireMandate(ctx context.Context, reference, by string) (Mandate, error) {
	if err := requireActor(by); err != nil {
		return Mandate{}, err
	}
	current, err := s.repo.GetMandate(ctx, reference)
	if err != nil {
		return Mandate{}, err
	}
	if _, err := current.Retire(); err != nil {
		return Mandate{}, err
	}
	retired, err := s.repo.RetireMandate(ctx, reference)
	if err != nil {
		return Mandate{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "mandate.retire",
		Subject: "mandate/" + retired.Reference,
		Before:  fmt.Sprintf("active=%t", current.Active),
		After:   fmt.Sprintf("active=%t", retired.Active),
	}); err != nil {
		return Mandate{}, err
	}
	return retired, nil
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
