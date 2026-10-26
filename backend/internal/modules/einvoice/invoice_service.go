package einvoice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
)

// ErrActorRequired is returned when the acting user is blank.
var ErrActorRequired = errors.New("einvoice: an actor is required")

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// InvoiceService implements the e-invoicing use cases. Every write is audited.
type InvoiceService struct {
	repo  InvoiceRepository
	audit AuditSink
}

// NewInvoiceService wires the service.
func NewInvoiceService(repo InvoiceRepository, sink AuditSink) *InvoiceService {
	return &InvoiceService{repo: repo, audit: sink}
}

// Create validates and stores an outbound e-invoice.
func (s *InvoiceService) Create(ctx context.Context, inv core.Invoice, by string) (core.Invoice, error) {
	if strings.TrimSpace(by) == "" {
		return core.Invoice{}, ErrActorRequired
	}
	if err := core.Validate(inv); err != nil {
		return core.Invoice{}, err
	}
	inv.CreatedBy = by
	saved, err := s.repo.Insert(ctx, inv)
	if err != nil {
		return core.Invoice{}, err
	}
	_, _, gross, _ := core.ComputeTotals(saved)
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "einvoice.create",
		Subject: fmt.Sprintf("e_invoice/%d", saved.ID),
		After:   fmt.Sprintf("number=%s gross_cents=%d", saved.Number, gross),
	}); err != nil {
		return core.Invoice{}, err
	}
	return saved, nil
}

// Get loads a stored invoice.
func (s *InvoiceService) Get(ctx context.Context, id int64) (core.Invoice, error) {
	return s.repo.Get(ctx, id)
}

// Render returns the invoice as an XRechnung 3.0 (UBL 2.1) document.
func (s *InvoiceService) Render(ctx context.Context, id int64) (string, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return core.GenerateXRechnung(inv)
}
