package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

type orderArgs struct {
	reference string
	year      int
	amount    int64
	currency  string
	recipient int64
	budget    int64
	purpose   string
	due       time.Time
	by        string
}

func validArgs() orderArgs {
	return orderArgs{
		reference: "AO-2026-0001",
		year:      2026,
		amount:    125000,
		currency:  "EUR",
		recipient: 1,
		budget:    1,
		purpose:   "Miete April",
		due:       time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		by:        "alice",
	}
}

func (a orderArgs) build() (PaymentOrder, error) {
	return NewPaymentOrder(a.reference, a.year, a.amount, a.currency, a.recipient,
		a.budget, a.purpose, a.due, a.by)
}

func TestNewPaymentOrderAcceptsValidInput(t *testing.T) {
	o, err := validArgs().build()
	if err != nil {
		t.Fatalf("valid order rejected: %v", err)
	}
	if o.Status != OrderDraft {
		t.Fatalf("status = %q, want draft", o.Status)
	}
	if o.CreatedBy != "alice" {
		t.Fatalf("createdBy = %q", o.CreatedBy)
	}
}

func TestNewPaymentOrderBoundaryYears(t *testing.T) {
	for _, year := range []int{1900, 2999} {
		a := validArgs()
		a.year = year
		if _, err := a.build(); err != nil {
			t.Fatalf("year %d must be accepted, got %v", year, err)
		}
	}
}

func TestNewPaymentOrderRejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func(*orderArgs)
	}{
		{"blank reference", ErrReferenceRequired, func(a *orderArgs) { a.reference = "   " }},
		{"year too low", ErrBadYear, func(a *orderArgs) { a.year = 1899 }},
		{"year too high", ErrBadYear, func(a *orderArgs) { a.year = 3000 }},
		{"zero amount", ErrAmountNotPositive, func(a *orderArgs) { a.amount = 0 }},
		{"negative amount", ErrAmountNotPositive, func(a *orderArgs) { a.amount = -5 }},
		{"short currency", ErrBadCurrency, func(a *orderArgs) { a.currency = "EU" }},
		{"long currency", ErrBadCurrency, func(a *orderArgs) { a.currency = "EURO" }},
		{"lowercase currency", ErrBadCurrency, func(a *orderArgs) { a.currency = "eur" }},
		{"missing recipient", ErrRecipientRequired, func(a *orderArgs) { a.recipient = 0 }},
		{"missing budget position", ErrBudgetPositionRequired, func(a *orderArgs) { a.budget = 0 }},
		{"blank purpose", ErrPurposeRequired, func(a *orderArgs) { a.purpose = "  " }},
		{"missing due date", ErrDueDateRequired, func(a *orderArgs) { a.due = time.Time{} }},
		{"missing actor", ErrActorRequired, func(a *orderArgs) { a.by = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := validArgs()
			c.mutate(&a)
			if _, err := a.build(); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestSubmit(t *testing.T) {
	base := PaymentOrder{ID: 1, Status: OrderDraft, CreatedBy: "alice"}

	got, err := base.Submit("alice")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Status != OrderSubmitted || got.SubmittedBy != "alice" {
		t.Fatalf("after submit: %+v", got)
	}

	if _, err := got.Submit("bob"); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("submitting twice: want ErrNotDraft, got %v", err)
	}
	if _, err := base.Submit("  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
}

// The four-eyes rule is the heart of S2: neither the creator nor the submitter
// may approve. This is what the legacy clone-based "approval" never enforced.
func TestApproveEnforcesFourEyes(t *testing.T) {
	samePerson := PaymentOrder{ID: 1, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice"}
	if _, err := samePerson.Approve("alice"); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("creator approving own order: want ErrFourEyes, got %v", err)
	}
	if _, err := samePerson.Approve("bob"); err != nil {
		t.Fatalf("independent approver must succeed: %v", err)
	}

	// Creator and submitter may differ: both are barred.
	mixed := PaymentOrder{ID: 2, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "carol"}
	if _, err := mixed.Approve("alice"); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("creator: want ErrFourEyes, got %v", err)
	}
	if _, err := mixed.Approve("carol"); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("submitter: want ErrFourEyes, got %v", err)
	}
	got, err := mixed.Approve("bob")
	if err != nil {
		t.Fatalf("independent approver: %v", err)
	}
	if got.Status != OrderApproved || got.DecidedBy != "bob" || !got.IsApproved() {
		t.Fatalf("after approve: %+v", got)
	}

	// Approving something that is not awaiting a decision is refused.
	draft := PaymentOrder{ID: 3, Status: OrderDraft, CreatedBy: "alice"}
	if _, err := draft.Approve("bob"); !errors.Is(err, ErrNotSubmitted) {
		t.Fatalf("draft: want ErrNotSubmitted, got %v", err)
	}
	if _, err := mixed.Approve("  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
}

func TestReject(t *testing.T) {
	submitted := PaymentOrder{ID: 1, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice"}

	got, err := submitted.Reject("bob", "  budget not available  ")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got.Status != OrderRejected || got.DecidedBy != "bob" {
		t.Fatalf("after reject: %+v", got)
	}
	if got.DecisionReason != "budget not available" {
		t.Fatalf("reason = %q, want it trimmed", got.DecisionReason)
	}

	if _, err := submitted.Reject("bob", "   "); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("blank reason: want ErrReasonRequired, got %v", err)
	}
	if _, err := submitted.Reject("  ", "why"); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
	draft := PaymentOrder{ID: 2, Status: OrderDraft}
	if _, err := draft.Reject("bob", "why"); !errors.Is(err, ErrNotSubmitted) {
		t.Fatalf("draft: want ErrNotSubmitted, got %v", err)
	}
}

func TestCancel(t *testing.T) {
	for _, status := range []OrderStatus{OrderDraft, OrderSubmitted} {
		got, err := PaymentOrder{ID: 1, Status: status}.Cancel("alice")
		if err != nil {
			t.Fatalf("cancel from %q: %v", status, err)
		}
		if got.Status != OrderCancelled || got.DecidedBy != "alice" {
			t.Fatalf("after cancel from %q: %+v", status, got)
		}
	}
	for _, status := range []OrderStatus{OrderApproved, OrderRejected, OrderCancelled} {
		if _, err := (PaymentOrder{ID: 1, Status: status}).Cancel("alice"); !errors.Is(err, ErrNotCancellable) {
			t.Fatalf("cancel from %q: want ErrNotCancellable, got %v", status, err)
		}
	}
	if _, err := (PaymentOrder{ID: 1, Status: OrderDraft}).Cancel(""); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
}

// ── service level ───────────────────────────────────────────────────────────

type fakeOrderRepo struct {
	order       PaymentOrder
	decisions   []OrderStatus
	actors      []string
	getErr      error
	insertErr   error
	saveErr     error
	recordErr   error
	recipient   Recipient
	budget      BudgetPosition
	createCalls int
	summaries   []OrderSummary
	lastStatus  OrderStatus
}

// ListOrders records the status it was asked for, so the service's pass-through is observable.
func (f *fakeOrderRepo) ListOrders(_ context.Context, status OrderStatus) ([]OrderSummary, error) {
	f.lastStatus = status
	return f.summaries, f.getErr
}

func (f *fakeOrderRepo) CreateRecipient(context.Context, Recipient) (Recipient, error) {
	return f.recipient, f.insertErr
}

func (f *fakeOrderRepo) CreateBudgetPosition(context.Context, BudgetPosition) (BudgetPosition, error) {
	return f.budget, f.insertErr
}

func (f *fakeOrderRepo) InsertOrder(_ context.Context, o PaymentOrder) (PaymentOrder, error) {
	if f.insertErr != nil {
		return PaymentOrder{}, f.insertErr
	}
	f.createCalls++
	o.ID = 7
	f.order = o
	return o, nil
}

func (f *fakeOrderRepo) GetOrder(context.Context, int64) (PaymentOrder, error) {
	if f.getErr != nil {
		return PaymentOrder{}, f.getErr
	}
	return f.order, nil
}

func (f *fakeOrderRepo) SaveOrderStatus(_ context.Context, o PaymentOrder, _ OrderStatus) (PaymentOrder, error) {
	if f.saveErr != nil {
		return PaymentOrder{}, f.saveErr
	}
	f.order = o
	return o, nil
}

func (f *fakeOrderRepo) RecordDecision(_ context.Context, _ int64, decision OrderStatus, actor, _ string) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.decisions = append(f.decisions, decision)
	f.actors = append(f.actors, actor)
	return nil
}

func TestOrderServiceWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := &fakeOrderRepo{}
	sink := &fakeAudit{}
	budget := &fakeBudget{}
	svc := NewOrderService(repo, budget, sink)

	order, err := validArgs().build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := svc.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != 7 {
		t.Fatalf("id = %d", created.ID)
	}

	if _, err := svc.Submit(ctx, 7, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	approved, err := svc.Approve(ctx, 7, "bob")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !approved.IsApproved() {
		t.Fatalf("not approved: %+v", approved)
	}

	if len(repo.decisions) != 2 {
		t.Fatalf("decisions = %v, want submitted + approved", repo.decisions)
	}
	if repo.decisions[0] != OrderSubmitted || repo.decisions[1] != OrderApproved {
		t.Fatalf("decision trail = %v", repo.decisions)
	}
	if len(sink.events) != 3 {
		t.Fatalf("audit events = %d, want 3 (create + submit + approve)", len(sink.events))
	}
	if sink.events[0].Action != "payment_order.create" || sink.events[2].Action != "payment_order.approve" {
		t.Fatalf("audit actions = %v", sink.events)
	}

	// The approval binds the amount on the order's budget position, named by the order.
	if len(budget.posted) != 1 {
		t.Fatalf("budget postings = %v, want the approval's binding", budget.posted)
	}
	got := budget.posted[0]
	if got.kind != "commitment" || got.amountCents != 125000 || got.reference != "ANORD:AO-2026-0001" {
		t.Fatalf("binding = %+v, want a commitment of 125000 named after the order", got)
	}
}

// TestOrderServiceApprovalBindsBudget: an approval that cannot bind budget is refused, and
// the order is not left approved.
func TestOrderServiceApprovalBindsBudget(t *testing.T) {
	ctx := context.Background()

	// No committer wired: approving would release money against no Mittel.
	repo := &fakeOrderRepo{order: PaymentOrder{ID: 7, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice"}}
	if _, err := NewOrderService(repo, nil, &fakeAudit{}).Approve(ctx, 7, "bob"); !errors.Is(err, ErrNoBudgetCommitter) {
		t.Fatalf("no committer: got %v, want ErrNoBudgetCommitter", err)
	}
	if len(repo.decisions) != 0 {
		t.Fatalf("a refused approval must not be recorded: %v", repo.decisions)
	}

	// The budget refuses the binding (over the position's Mittel): the approval fails too.
	repo = &fakeOrderRepo{order: PaymentOrder{ID: 7, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice", AmountCents: 999}}
	refusing := &fakeBudget{err: errors.New("budget: insufficient")}
	if _, err := NewOrderService(repo, refusing, &fakeAudit{}).Approve(ctx, 7, "bob"); err == nil {
		t.Fatal("an un-bindable approval must fail")
	}
	if len(repo.decisions) != 0 {
		t.Fatalf("a failed binding must not record an approval: %v", repo.decisions)
	}
}

// fakeBudget records the bindings a service posts.
type fakeBudget struct {
	posted []budgetPosting
	err    error
}

type budgetPosting struct {
	positionID  int64
	kind        string
	amountCents int64
	reference   string
}

func (f *fakeBudget) Post(_ context.Context, positionID int64, kind budget.PostingKind, amountCents int64, reference, _ string) (budget.Posting, error) {
	if f.err != nil {
		return budget.Posting{}, f.err
	}
	f.posted = append(f.posted, budgetPosting{positionID: positionID, kind: string(kind), amountCents: amountCents, reference: reference})
	return budget.Posting{}, nil
}

func TestOrderServiceBlocksFourEyes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeOrderRepo{order: PaymentOrder{ID: 7, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice"}}
	svc := NewOrderService(repo, &fakeBudget{}, &fakeAudit{})

	if _, err := svc.Approve(ctx, 7, "alice"); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("want ErrFourEyes, got %v", err)
	}
	if len(repo.decisions) != 0 {
		t.Fatalf("a blocked approval must not be recorded: %v", repo.decisions)
	}
}

func TestOrderServicePropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	if _, err := NewOrderService(&fakeOrderRepo{insertErr: boom}, &fakeBudget{}, &fakeAudit{}).Create(ctx, PaymentOrder{}); !errors.Is(err, boom) {
		t.Fatalf("Create: want boom, got %v", err)
	}
	if _, err := NewOrderService(&fakeOrderRepo{getErr: boom}, &fakeBudget{}, &fakeAudit{}).Submit(ctx, 1, "a"); !errors.Is(err, boom) {
		t.Fatalf("Submit: want boom, got %v", err)
	}
	if _, err := NewOrderService(&fakeOrderRepo{order: PaymentOrder{ID: 1, Status: OrderDraft}, saveErr: boom}, &fakeBudget{}, &fakeAudit{}).Submit(ctx, 1, "a"); !errors.Is(err, boom) {
		t.Fatalf("Submit save: want boom, got %v", err)
	}
	if _, err := NewOrderService(&fakeOrderRepo{order: PaymentOrder{ID: 1, Status: OrderDraft}, recordErr: boom}, &fakeBudget{}, &fakeAudit{}).Submit(ctx, 1, "a"); !errors.Is(err, boom) {
		t.Fatalf("Submit record: want boom, got %v", err)
	}

	// An audit failure after a successful transition must surface.
	repo := &fakeOrderRepo{order: PaymentOrder{ID: 1, Status: OrderDraft, CreatedBy: "alice"}}
	svc := NewOrderService(repo, &fakeBudget{}, failingAudit{err: boom})
	if _, err := svc.Submit(ctx, 1, "alice"); !errors.Is(err, boom) {
		t.Fatalf("Submit audit: want boom, got %v", err)
	}
}

func TestOrderServiceRejectAndCancel(t *testing.T) {
	ctx := context.Background()
	repo := &fakeOrderRepo{order: PaymentOrder{ID: 1, Status: OrderSubmitted, CreatedBy: "alice", SubmittedBy: "alice"}}
	svc := NewOrderService(repo, &fakeBudget{}, &fakeAudit{})

	rejected, err := svc.Reject(ctx, 1, "bob", "kein Budget")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if rejected.Status != OrderRejected || !strings.Contains(rejected.DecisionReason, "Budget") {
		t.Fatalf("after reject: %+v", rejected)
	}

	repo.order = PaymentOrder{ID: 2, Status: OrderDraft}
	cancelled, err := svc.Cancel(ctx, 2, "alice")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != OrderCancelled {
		t.Fatalf("after cancel: %+v", cancelled)
	}

	got, err := svc.Order(ctx, 2)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	if got.ID != 2 {
		t.Fatalf("got id %d", got.ID)
	}
}

// TestOrdersPassesTheStatusThrough: the service adds nothing to the read, and the filter the
// caller named is the one the store is asked for.
func TestOrdersPassesTheStatusThrough(t *testing.T) {
	ctx := context.Background()
	repo := &fakeOrderRepo{summaries: []OrderSummary{{ID: 1, Reference: "AO-1", Status: OrderDraft}}}
	svc := NewOrderService(repo, nil, &fakeAudit{})

	got, err := svc.Orders(ctx, OrderSubmitted)
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	if len(got) != 1 || got[0].Reference != "AO-1" {
		t.Fatalf("orders = %+v", got)
	}
	if repo.lastStatus != OrderSubmitted {
		t.Fatalf("the store was asked for %q", repo.lastStatus)
	}

	boom := errors.New("boom")
	if _, err := NewOrderService(&fakeOrderRepo{getErr: boom}, nil, &fakeAudit{}).Orders(ctx, ""); !errors.Is(err, boom) {
		t.Fatalf("store error = %v, want boom", err)
	}
}
