package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// OrderService implements the payment-order workflow (S2).
type OrderService struct {
	repo  OrderRepository
	audit AuditSink
}

// NewOrderService wires the payment-order service.
func NewOrderService(repo OrderRepository, sink AuditSink) *OrderService {
	return &OrderService{repo: repo, audit: sink}
}

// CreateRecipient stores a payment recipient.
func (s *OrderService) CreateRecipient(ctx context.Context, r Recipient) (Recipient, error) {
	return s.repo.CreateRecipient(ctx, r)
}

// CreateBudgetPosition stores an accounting dimension.
func (s *OrderService) CreateBudgetPosition(ctx context.Context, b BudgetPosition) (BudgetPosition, error) {
	return s.repo.CreateBudgetPosition(ctx, b)
}

// Create validates and stores a new draft order.
func (s *OrderService) Create(ctx context.Context, o PaymentOrder) (PaymentOrder, error) {
	saved, err := s.repo.InsertOrder(ctx, o)
	if err != nil {
		return PaymentOrder{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   o.CreatedBy,
		Action:  "payment_order.create",
		Subject: orderSubject(saved.ID),
		After:   fmt.Sprintf("reference=%s amount_cents=%d status=%s", saved.Reference, saved.AmountCents, saved.Status),
	}); err != nil {
		return PaymentOrder{}, err
	}
	return saved, nil
}

// Order loads an order.
func (s *OrderService) Order(ctx context.Context, id int64) (PaymentOrder, error) {
	return s.repo.GetOrder(ctx, id)
}

// Submit moves a draft into the approval queue.
func (s *OrderService) Submit(ctx context.Context, id int64, by string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.submit", func(o PaymentOrder) (PaymentOrder, error) {
		return o.Submit(by)
	})
}

// Approve releases a submitted order under the four-eyes principle.
func (s *OrderService) Approve(ctx context.Context, id int64, by string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.approve", func(o PaymentOrder) (PaymentOrder, error) {
		return o.Approve(by)
	})
}

// Reject returns a submitted order with a reason.
func (s *OrderService) Reject(ctx context.Context, id int64, by, reason string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.reject", func(o PaymentOrder) (PaymentOrder, error) {
		return o.Reject(by, reason)
	})
}

// Cancel withdraws a draft or submitted order.
func (s *OrderService) Cancel(ctx context.Context, id int64, by string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.cancel", func(o PaymentOrder) (PaymentOrder, error) {
		return o.Cancel(by)
	})
}

// apply runs one workflow transition: load, decide, persist, record, audit.
func (s *OrderService) apply(ctx context.Context, id int64, actor, action string,
	fn func(PaymentOrder) (PaymentOrder, error)) (PaymentOrder, error) {

	current, err := s.repo.GetOrder(ctx, id)
	if err != nil {
		return PaymentOrder{}, err
	}
	next, err := fn(current)
	if err != nil {
		return PaymentOrder{}, err
	}
	saved, err := s.repo.SaveOrderStatus(ctx, next, current.Status)
	if err != nil {
		return PaymentOrder{}, err
	}
	if err := s.repo.RecordDecision(ctx, id, next.Status, actor, next.DecisionReason); err != nil {
		return PaymentOrder{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   actor,
		Action:  action,
		Subject: orderSubject(id),
		After:   "status=" + string(next.Status),
	}); err != nil {
		return PaymentOrder{}, err
	}
	return saved, nil
}

func orderSubject(id int64) string { return fmt.Sprintf("order/%d", id) }
