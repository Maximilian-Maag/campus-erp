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
