package procurement

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// BudgetCommitter commits or releases budget for a position. The budget module's
// PositionService satisfies it.
type BudgetCommitter interface {
	Post(ctx context.Context, positionID int64, kind budget.PostingKind, amountCents int64, reference, by string) (budget.Posting, error)
}

// OrderService implements the procurement use cases. Every write is audited.
type OrderService struct {
	repo   OrderRepository
	budget BudgetCommitter
	audit  AuditSink
}

// NewOrderService wires the service.
func NewOrderService(repo OrderRepository, committer BudgetCommitter, sink AuditSink) *OrderService {
	return &OrderService{repo: repo, budget: committer, audit: sink}
}

// Create validates and stores a draft order.
func (s *OrderService) Create(ctx context.Context, number, supplier string, budgetPositionID int64, currency string, lines []Line, by string) (Order, error) {
	if strings.TrimSpace(by) == "" {
		return Order{}, ErrActorRequired
	}
	order, err := NewOrder(number, supplier, budgetPositionID, currency, lines)
	if err != nil {
		return Order{}, err
	}
	order.CreatedBy = by
	saved, err := s.repo.Insert(ctx, order)
	if err != nil {
		return Order{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "purchase_order.create",
		Subject: orderSubject(saved.ID),
		After:   fmt.Sprintf("number=%s gross_cents=%d", saved.Number, saved.GrossCents),
	}); err != nil {
		return Order{}, err
	}
	return saved, nil
}

// Approve moves a draft to approved and commits the gross amount to the budget.
func (s *OrderService) Approve(ctx context.Context, id int64, by string) (Order, error) {
	return s.transition(ctx, id, by, StatusDraft, StatusApproved, "purchase_order.approve", func(o Order) error {
		_, err := s.budget.Post(ctx, o.BudgetPositionID, budget.Commitment, o.GrossCents, "PO:"+o.Number, by)
		return err
	})
}

// Receive moves an approved order to received.
func (s *OrderService) Receive(ctx context.Context, id int64, by string) (Order, error) {
	return s.transition(ctx, id, by, StatusApproved, StatusReceived, "purchase_order.receive", nil)
}

// Orders returns the order summaries, optionally narrowed to one status ("" for every status).
// It is read-only, so it is not audited.
func (s *OrderService) Orders(ctx context.Context, status Status, limit, offset int) ([]OrderSummary, error) {
	return s.repo.List(ctx, status, limit, offset)
}

// MatchReceipt is the two-way match: it finds the order an inbound invoice names (its buyer
// reference is the order number), requires that order to be approved, and converts what the invoice
// bills from the commitment into an actual.
//
// An invoice may bill the order in parts (docs/12): it converts its own amount, the order records
// how much has been invoiced, and it becomes `received` only when nothing is left. An invoice that
// bills more than is left is refused, so the commitment is never over-converted.
//
// The commitment is released and the actual posted in one step, so the position's available Mittel
// are unchanged by the conversion rather than being counted twice (docs/10). A refusal leaves the
// order and the budget untouched.
func (s *OrderService) MatchReceipt(ctx context.Context, orderNumber string, grossCents int64, reference, by string) (Order, error) {
	if strings.TrimSpace(by) == "" {
		return Order{}, ErrActorRequired
	}
	if grossCents <= 0 {
		return Order{}, fmt.Errorf("%w: invoice=%d", ErrMatchAmountNotPositive, grossCents)
	}
	order, err := s.repo.GetByNumber(ctx, strings.TrimSpace(orderNumber))
	if err != nil {
		return Order{}, err
	}
	if order.Status != StatusApproved {
		return Order{}, fmt.Errorf("%w: order %s is %s", ErrBadTransition, order.Number, order.Status)
	}
	if grossCents > order.GrossCents {
		return Order{}, fmt.Errorf("%w: order=%d invoice=%d", ErrMatchAmountMismatch, order.GrossCents, grossCents)
	}
	// Deliveries and earlier invoices may have converted part of the commitment already (docs/12).
	// An invoice may state up to the order's gross — a supplier bills the order, not the remainder —
	// so it converts what is *left*: a full-gross invoice after a partial delivery converts only the
	// remainder, and a partial invoice converts only what it bills.
	_, converted, err := s.repo.GoodsReceived(ctx, order.ID)
	if err != nil {
		return Order{}, err
	}
	remaining := remainingCommitment(order.GrossCents, converted+order.InvoicedCents)
	if remaining == 0 {
		return Order{}, fmt.Errorf("%w: order %s has no commitment left to convert", ErrBadTransition, order.Number)
	}
	convert := grossCents
	if convert > remaining {
		convert = remaining
	}
	if _, err := s.budget.Post(ctx, order.BudgetPositionID, budget.Release, convert, reference+":release", by); err != nil {
		return Order{}, err
	}
	if _, err := s.budget.Post(ctx, order.BudgetPositionID, budget.Actual, convert, reference, by); err != nil {
		return Order{}, err
	}
	saved, err := s.repo.AddInvoiced(ctx, order.ID, convert)
	if err != nil {
		return Order{}, err
	}
	// Only the invoice that completes the order closes it; a partial invoice leaves it open for the
	// rest, which is the whole point of billing in parts.
	if convert == remaining {
		saved, err = s.repo.SetStatus(ctx, order.ID, StatusReceived, by)
		if err != nil {
			return Order{}, err
		}
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "purchase_order.match",
		Subject: orderSubject(order.ID),
		After: fmt.Sprintf("number=%s gross_cents=%d invoiced_cents=%d converted_cents=%d remaining_cents=%d invoice=%s status=%s",
			saved.Number, saved.GrossCents, saved.InvoicedCents, convert, remaining-convert, reference, saved.Status),
	}); err != nil {
		return Order{}, err
	}
	return saved, nil
}

// Cancel abandons an order; an approved order's commitment is released.
func (s *OrderService) Cancel(ctx context.Context, id int64, by string) (Order, error) {
	if strings.TrimSpace(by) == "" {
		return Order{}, ErrActorRequired
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if current.Status == StatusCancelled || current.Status == StatusReceived {
		return Order{}, ErrBadTransition
	}
	if current.Status == StatusApproved {
		if _, err := s.budget.Post(ctx, current.BudgetPositionID, budget.Release, current.GrossCents, "PO:"+current.Number+":cancel", by); err != nil {
			return Order{}, err
		}
	}
	saved, err := s.repo.SetStatus(ctx, id, StatusCancelled, by)
	if err != nil {
		return Order{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "purchase_order.cancel",
		Subject: orderSubject(id),
		After:   fmt.Sprintf("number=%s", saved.Number),
	}); err != nil {
		return Order{}, err
	}
	return saved, nil
}

// Get loads an order.
func (s *OrderService) Get(ctx context.Context, id int64) (Order, error) {
	return s.repo.Get(ctx, id)
}

// transition applies a status change guarded by the required current status.
func (s *OrderService) transition(ctx context.Context, id int64, by string, from, to Status, action string, side func(Order) error) (Order, error) {
	if strings.TrimSpace(by) == "" {
		return Order{}, ErrActorRequired
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if current.Status != from {
		return Order{}, ErrBadTransition
	}
	if side != nil {
		if err := side(current); err != nil {
			return Order{}, err
		}
	}
	saved, err := s.repo.SetStatus(ctx, id, to, by)
	if err != nil {
		return Order{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  action,
		Subject: orderSubject(id),
		After:   fmt.Sprintf("number=%s status=%s", saved.Number, saved.Status),
	}); err != nil {
		return Order{}, err
	}
	return saved, nil
}

func orderSubject(id int64) string { return fmt.Sprintf("purchase_order/%d", id) }
