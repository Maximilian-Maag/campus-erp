package einvoice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// ErrActorRequired is returned when the acting user is blank.
var ErrActorRequired = errors.New("einvoice: an actor is required")

// ErrNoOrderMatcher is returned when an invoice cannot be matched because the service was
// wired without a matcher. Matching against nothing would leave the invoice silently
// unmatched, so it is refused rather than skipped.
var ErrNoOrderMatcher = errors.New("einvoice: no purchase-order matcher configured")

// OrderMatcher runs the two-way match for an invoice: it finds the purchase order the invoice
// names, checks the amounts agree, and turns that order's commitment into an actual. The
// procurement module's OrderService satisfies it.
type OrderMatcher interface {
	MatchReceipt(ctx context.Context, orderNumber string, grossCents int64, reference, by string) (procurement.Order, error)
}

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// InvoiceService implements the e-invoicing use cases. Every write is audited.
type InvoiceService struct {
	repo   InvoiceRepository
	orders OrderMatcher
	audit  AuditSink
}

// NewInvoiceService wires the service. orders may be nil until an invoice is matched, but a
// match then fails loudly (ErrNoOrderMatcher).
func NewInvoiceService(repo InvoiceRepository, orders OrderMatcher, sink AuditSink) *InvoiceService {
	return &InvoiceService{repo: repo, orders: orders, audit: sink}
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
	inv.Direction = core.Outbound
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

// Invoices returns the invoices as summaries, ordered by number. It is read-only.
func (s *InvoiceService) Invoices(ctx context.Context, limit, offset int) ([]InvoiceSummary, error) {
	return s.repo.List(ctx, limit, offset)
}

// Match runs the two-way match for an inbound invoice (docs/09, docs/12): the order its buyer
// reference names must exist, be approved and amount to the same gross, and the match converts
// that order's commitment into an actual. The invoice records the match on the audit stream.
//
// The buyer reference is the key because BT-10 is what a public buyer puts on the order and
// the supplier echoes back; an invoice without one cannot be matched.
func (s *InvoiceService) Match(ctx context.Context, id int64, by string) (core.Invoice, error) {
	if strings.TrimSpace(by) == "" {
		return core.Invoice{}, ErrActorRequired
	}
	if s.orders == nil {
		return core.Invoice{}, ErrNoOrderMatcher
	}
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return core.Invoice{}, err
	}
	if inv.Direction != core.Inbound {
		return core.Invoice{}, ErrNotInbound
	}
	if inv.MatchedOrderID != 0 {
		return core.Invoice{}, ErrAlreadyMatched
	}
	reference := strings.TrimSpace(inv.BuyerReference)
	if reference == "" {
		return core.Invoice{}, core.ErrBuyerReference
	}
	_, _, gross, _ := core.ComputeTotals(inv)
	order, err := s.orders.MatchReceipt(ctx, reference, gross, "RE:"+inv.Number, by)
	if err != nil {
		return core.Invoice{}, err
	}
	// The order is the authority and has already moved; recording it on the invoice is what lets
	// a screen tell a matched invoice from an unmatched one. If this write fails the order stays
	// matched and this call fails loudly rather than reporting a match that is not recorded.
	if err := s.repo.MarkMatched(ctx, inv.ID, order.ID); err != nil {
		return core.Invoice{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "einvoice.match",
		Subject: fmt.Sprintf("e_invoice/%d", inv.ID),
		After:   fmt.Sprintf("number=%s order=%s order_id=%d gross_cents=%d", inv.Number, order.Number, order.ID, gross),
	}); err != nil {
		return core.Invoice{}, err
	}
	return inv, nil
}

// ImportReceived parses, validates and stores an inbound XRechnung document.
func (s *InvoiceService) ImportReceived(ctx context.Context, doc, by string) (core.Invoice, error) {
	if strings.TrimSpace(by) == "" {
		return core.Invoice{}, ErrActorRequired
	}
	inv, err := core.ParseReceived(doc)
	if err != nil {
		return core.Invoice{}, err
	}
	inv.CreatedBy = by
	inv.Direction = core.Inbound
	saved, err := s.repo.Insert(ctx, inv)
	if err != nil {
		return core.Invoice{}, err
	}
	_, _, gross, _ := core.ComputeTotals(saved)
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "einvoice.receive",
		Subject: fmt.Sprintf("e_invoice/%d", saved.ID),
		After:   fmt.Sprintf("number=%s seller=%s gross_cents=%d", saved.Number, saved.Seller.Name, gross),
	}); err != nil {
		return core.Invoice{}, err
	}
	return saved, nil
}

// Render returns the invoice as an XRechnung 3.0 (UBL 2.1) document.
func (s *InvoiceService) Render(ctx context.Context, id int64) (string, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return core.GenerateXRechnung(inv)
}

// RenderCII renders the same stored invoice as ZUGFeRD/Factur-X (CII). It is a second syntax over
// one model, so it validates the same way and refuses the same invoices (docs/09).
func (s *InvoiceService) RenderCII(ctx context.Context, id int64) (string, error) {
	inv, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return core.GenerateCII(inv)
}
