package kasse

import (
	"context"
	"fmt"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// ReceivableService records and reads the open items a collection run gathers.
type ReceivableService struct {
	repo  ReceivableRepository
	audit AuditSink
}

// NewReceivableService wires the receivable service.
func NewReceivableService(repo ReceivableRepository, sink AuditSink) *ReceivableService {
	return &ReceivableService{repo: repo, audit: sink}
}

// Post records an open receivable. It is the only way an item enters the books;
// a run then selects from what is open.
func (s *ReceivableService) Post(ctx context.Context, mandateReference string, amountCents int64,
	currency string, dueDate time.Time, purpose, source, by string) (Receivable, error) {

	if err := requireActor(by); err != nil {
		return Receivable{}, err
	}
	rec, err := NewReceivable(mandateReference, amountCents, currency, dueDate, purpose, source)
	if err != nil {
		return Receivable{}, err
	}
	saved, err := s.repo.CreateReceivable(ctx, rec, by)
	if err != nil {
		return Receivable{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "receivable.create",
		Subject: receivableSubject(saved.ID),
		After: fmt.Sprintf("mandate=%s amount_cents=%d currency=%s due=%s source=%s",
			saved.MandateReference, saved.AmountCents, saved.Currency, saved.DueDate.Format(time.DateOnly), saved.Source),
	}); err != nil {
		return Receivable{}, err
	}
	return saved, nil
}

// Receivable loads an open item by id.
func (s *ReceivableService) Receivable(ctx context.Context, id int64) (Receivable, error) {
	return s.repo.GetReceivable(ctx, id)
}

// Receivables lists open items, due date first, optionally narrowed to one state.
func (s *ReceivableService) Receivables(ctx context.Context, state ReceivableState) ([]Receivable, error) {
	return s.repo.Receivables(ctx, state)
}

func receivableSubject(id int64) string { return fmt.Sprintf("receivable/%d", id) }
