package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// BudgetCommitter binds and releases Mittel on a budget position. The budget module's
// PositionService satisfies it.
type BudgetCommitter interface {
	Post(ctx context.Context, positionID int64, kind budget.PostingKind, amountCents int64, reference, by string) (budget.Posting, error)
}

// OrderService implements the payment-order workflow (S2).
type OrderService struct {
	repo   OrderRepository
	budget BudgetCommitter
	audit  AuditSink
}

// NewOrderService wires the payment-order service.
func NewOrderService(repo OrderRepository, committer BudgetCommitter, sink AuditSink) *OrderService {
	return &OrderService{repo: repo, budget: committer, audit: sink}
}

// CreateRecipient stores a payment recipient.
func (s *OrderService) CreateRecipient(ctx context.Context, r Recipient) (Recipient, error) {
	return s.repo.CreateRecipient(ctx, r)
}

// CreateBudgetPosition stores an accounting dimension with the Mittel it is established with.
func (s *OrderService) CreateBudgetPosition(ctx context.Context, b BudgetPosition) (BudgetPosition, error) {
	if b.AllocatedCents < 0 {
		return BudgetPosition{}, ErrAllocationNegative
	}
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
// Orders lists payment orders, optionally narrowed to one status.
func (s *OrderService) Orders(ctx context.Context, status OrderStatus) ([]OrderSummary, error) {
	return s.repo.ListOrders(ctx, status)
}

func (s *OrderService) Order(ctx context.Context, id int64) (PaymentOrder, error) {
	return s.repo.GetOrder(ctx, id)
}

// Submit moves a draft into the approval queue.
func (s *OrderService) Submit(ctx context.Context, id int64, by string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.submit", func(o PaymentOrder) (PaymentOrder, error) {
		return o.Submit(by)
	})
}

// Approve releases a submitted order under the four-eyes principle and binds its amount on
// the budget position it names (Mittelbindung): an approved order has committed budget.
//
// The commitment is posted BEFORE the order is saved, so an approval that cannot be bound
// does not leave the order approved. The budget module refuses a binding that exceeds the
// position's available Mittel (ErrBudgetExceeded), so an order cannot be approved beyond its
// budget. There is no release path here because an approved order cannot be cancelled
// (ErrNotCancellable): withdrawing one is a reversal, which is not modelled yet.
func (s *OrderService) Approve(ctx context.Context, id int64, by string) (PaymentOrder, error) {
	return s.apply(ctx, id, by, "payment_order.approve", func(o PaymentOrder) (PaymentOrder, error) {
		approved, err := o.Approve(by)
		if err != nil {
			// Four-eyes and state errors come first: they are decided before anything is bound.
			return PaymentOrder{}, err
		}
		if s.budget == nil {
			return PaymentOrder{}, ErrNoBudgetCommitter
		}
		if _, err := s.budget.Post(ctx, approved.BudgetPositionID, budget.Commitment,
			approved.AmountCents, orderCommitmentReference(approved.Reference), by); err != nil {
			return PaymentOrder{}, err
		}
		return approved, nil
	})
}

// orderCommitmentReference names the economic event a binding belongs to, so a position's
// postings can be read back to the order that bound them.
func orderCommitmentReference(reference string) string { return "ANORD:" + reference }

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
