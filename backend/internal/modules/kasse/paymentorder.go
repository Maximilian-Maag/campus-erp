package kasse

import (
	"errors"
	"strings"
	"time"
)

// OrderStatus is the workflow state of a payment order (Anordnung).
type OrderStatus string

const (
	OrderDraft     OrderStatus = "draft"
	OrderSubmitted OrderStatus = "submitted"
	OrderApproved  OrderStatus = "approved"
	OrderRejected  OrderStatus = "rejected"
	OrderCancelled OrderStatus = "cancelled"
)

var (
	// ErrNotDraft is returned when an order must be a draft but is not.
	ErrNotDraft = errors.New("kasse: order is not a draft")
	// ErrNotSubmitted is returned when an order is not awaiting a decision.
	ErrNotSubmitted = errors.New("kasse: order is not awaiting a decision")
	// ErrFourEyes is returned when the approver created or submitted the order.
	ErrFourEyes = errors.New("kasse: four-eyes violation: the approver must not be the creator or submitter")
	// ErrReasonRequired is returned when a rejection has no reason.
	ErrReasonRequired = errors.New("kasse: a reason is required")
	// ErrActorRequired is returned when the acting user is missing.
	ErrActorRequired = errors.New("kasse: an acting user is required")
	// ErrNotCancellable is returned when an approved order is cancelled.
	ErrNotCancellable = errors.New("kasse: an approved order cannot be cancelled, it needs a reversal")
	// ErrReferenceRequired is returned when an order has no reference.
	ErrReferenceRequired = errors.New("kasse: a reference is required")
	// ErrBadYear is returned for an implausible business year.
	ErrBadYear = errors.New("kasse: implausible business year")
	// ErrBadCurrency is returned for a currency that is not an ISO-4217 code.
	ErrBadCurrency = errors.New("kasse: currency must be a three-letter ISO-4217 code")
	// ErrRecipientRequired is returned when the recipient is missing.
	ErrRecipientRequired = errors.New("kasse: a recipient is required")
	// ErrBudgetPositionRequired is returned when the budget position is missing.
	ErrBudgetPositionRequired = errors.New("kasse: a budget position is required")
	// ErrDueDateRequired is returned when the due date is missing.
	ErrDueDateRequired = errors.New("kasse: a due date is required")
	// ErrOrderConflict is returned when an order changed between read and write.
	ErrOrderConflict = errors.New("kasse: the order was modified concurrently")
	// ErrReferenceExists is returned when the reference is already in use.
	ErrReferenceExists = errors.New("kasse: an order with that reference already exists")
	// ErrAllocationNegative is returned for a negative Mittel allocation.
	ErrAllocationNegative = errors.New("kasse: a budget allocation cannot be negative")
	// ErrNoBudgetCommitter is returned when an approval cannot bind budget because the
	// service was wired without a committer. Approving without binding would approve money
	// against no Mittel, so it is refused rather than skipped.
	ErrNoBudgetCommitter = errors.New("kasse: no budget committer configured")
)

// PaymentOrder is a canonical payment order.
type PaymentOrder struct {
	ID               int64
	Reference        string
	BusinessYear     int
	AmountCents      int64
	Currency         string
	RecipientID      int64
	BudgetPositionID int64
	Purpose          string
	DueDate          time.Time
	Status           OrderStatus
	CreatedBy        string
	SubmittedBy      string
	DecidedBy        string
	DecisionReason   string
}

// NewPaymentOrder validates and builds a draft payment order.
func NewPaymentOrder(reference string, businessYear int, amountCents int64, currency string,
	recipientID, budgetPositionID int64, purpose string, dueDate time.Time, createdBy string) (PaymentOrder, error) {

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return PaymentOrder{}, ErrReferenceRequired
	}
	if businessYear < 1900 || businessYear > 2999 {
		return PaymentOrder{}, ErrBadYear
	}
	if amountCents <= 0 {
		return PaymentOrder{}, ErrAmountNotPositive
	}
	if len(currency) != 3 || currency != strings.ToUpper(currency) {
		return PaymentOrder{}, ErrBadCurrency
	}
	if recipientID <= 0 {
		return PaymentOrder{}, ErrRecipientRequired
	}
	if budgetPositionID <= 0 {
		return PaymentOrder{}, ErrBudgetPositionRequired
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return PaymentOrder{}, ErrPurposeRequired
	}
	if dueDate.IsZero() {
		return PaymentOrder{}, ErrDueDateRequired
	}
	if strings.TrimSpace(createdBy) == "" {
		return PaymentOrder{}, ErrActorRequired
	}

	return PaymentOrder{
		Reference:        reference,
		BusinessYear:     businessYear,
		AmountCents:      amountCents,
		Currency:         currency,
		RecipientID:      recipientID,
		BudgetPositionID: budgetPositionID,
		Purpose:          purpose,
		DueDate:          dueDate,
		Status:           OrderDraft,
		CreatedBy:        createdBy,
	}, nil
}

// Submit moves a draft into the approval queue.
func (o PaymentOrder) Submit(by string) (PaymentOrder, error) {
	if o.Status != OrderDraft {
		return PaymentOrder{}, ErrNotDraft
	}
	if strings.TrimSpace(by) == "" {
		return PaymentOrder{}, ErrActorRequired
	}
	o.Status = OrderSubmitted
	o.SubmittedBy = by
	return o, nil
}

// Approve applies the four-eyes principle: whoever approves must be neither the
// creator nor the submitter of the order.
func (o PaymentOrder) Approve(by string) (PaymentOrder, error) {
	if o.Status != OrderSubmitted {
		return PaymentOrder{}, ErrNotSubmitted
	}
	if strings.TrimSpace(by) == "" {
		return PaymentOrder{}, ErrActorRequired
	}
	if by == o.CreatedBy || by == o.SubmittedBy {
		return PaymentOrder{}, ErrFourEyes
	}
	o.Status = OrderApproved
	o.DecidedBy = by
	return o, nil
}

// Reject returns a submitted order with a mandatory reason.
func (o PaymentOrder) Reject(by, reason string) (PaymentOrder, error) {
	if o.Status != OrderSubmitted {
		return PaymentOrder{}, ErrNotSubmitted
	}
	if strings.TrimSpace(by) == "" {
		return PaymentOrder{}, ErrActorRequired
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return PaymentOrder{}, ErrReasonRequired
	}
	o.Status = OrderRejected
	o.DecidedBy = by
	o.DecisionReason = reason
	return o, nil
}

// Cancel withdraws a draft or a submitted order. An approved order is not
// cancellable: it has to be reversed by a booking.
func (o PaymentOrder) Cancel(by string) (PaymentOrder, error) {
	if o.Status != OrderDraft && o.Status != OrderSubmitted {
		return PaymentOrder{}, ErrNotCancellable
	}
	if strings.TrimSpace(by) == "" {
		return PaymentOrder{}, ErrActorRequired
	}
	o.Status = OrderCancelled
	o.DecidedBy = by
	return o, nil
}

// IsApproved reports whether the order has been released.
func (o PaymentOrder) IsApproved() bool { return o.Status == OrderApproved }

// Recipient is a payment recipient (Zahlungspartner).
type Recipient struct {
	ID            int64
	Name          string
	AccountHolder string
	IBAN          string
	BIC           string
	BankName      string
}

// BudgetPosition is the accounting dimension of an order.
type BudgetPosition struct {
	ID          int64
	Year        int
	Chapter     string
	Title       string
	CostCentre  string
	CostCarrier string
	// AllocatedCents is the Mittel (Soll) the position is established with. It applies when
	// the position is first created; naming an existing position does not rewrite its Ansatz,
	// because an order may not change the budget it draws on.
	AllocatedCents int64
}
