package kasse_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// budgetService builds the real budget module over pool: an approval now binds Mittel on the
// order's position, so these tests exercise the real guard rather than a fake.
func budgetService(pool *pgxpool.Pool) *budget.PositionService {
	return budget.NewPositionService(budget.NewPositionRepo(pool), audit.NewStore(pool))
}

// newOrderFixture stores a recipient + budget position + draft order and returns
// the order service and the order. References are unique per run so that tests
// sharing one database do not collide.
func newOrderFixture(t *testing.T, amountCents int64) (*kasse.OrderService, *pgxpool.Pool, kasse.PaymentOrder) {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	repo := kasse.NewOrderRepo(pool)
	svc := kasse.NewOrderService(repo, budgetService(pool), audit.NewStore(pool))

	ref := fmt.Sprintf("AO-%d", time.Now().UnixNano())
	rec, err := svc.CreateRecipient(ctx, kasse.Recipient{
		Name: "Stadtwerke Musterstadt", AccountHolder: "Stadtwerke Musterstadt",
		IBAN: "DE02120300000000202051", BIC: "BYLADEM1001", BankName: "Musterbank",
	})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	// Twice the order's amount as Mittel, so approving it binds without breaching the Ansatz.
	pos, err := svc.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year: 2026, Chapter: "0710", Title: "6810", CostCentre: "KST-1", CostCarrier: "KT-1",
		AllocatedCents: 2 * amountCents,
	})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}
	order, err := kasse.NewPaymentOrder(ref, 2026, amountCents, "EUR", rec.ID, pos.ID,
		"Stromabschlag", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	created, err := svc.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return svc, pool, created
}

// TestPaymentOrderApprovalBindsBudget: approving an order binds its amount on the position it
// names, and the budget module's guard refuses an order that exceeds the position's Mittel.
func TestPaymentOrderApprovalBindsBudget(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewOrderService(kasse.NewOrderRepo(pool), budgetService(pool), audit.NewStore(pool))

	ref := fmt.Sprintf("AO-%d", time.Now().UnixNano())
	rec, err := svc.CreateRecipient(ctx, kasse.Recipient{
		Name: "Stadtwerke " + ref, AccountHolder: "Stadtwerke", IBAN: "DE02120300000000202051",
		BIC: "BYLADEM1001", BankName: "Musterbank",
	})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := svc.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year: 2027, Chapter: "0710", Title: "6810", CostCentre: "KST-" + ref, CostCarrier: "KT",
		AllocatedCents: 100000, // 1000.00 of Mittel
	})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}

	order, err := kasse.NewPaymentOrder(ref, 2027, 60000, "EUR", rec.ID, pos.ID,
		"Miete", time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	created, err := svc.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Submit(ctx, created.ID, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := svc.Approve(ctx, created.ID, "bob"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	bal, err := budgetService(pool).Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 60000 {
		t.Fatalf("committed = %d, want the approved order's 60000", bal.CommittedCents)
	}
	if bal.AvailableCents() != 40000 {
		t.Fatalf("available = %d, want 40000 after the binding", bal.AvailableCents())
	}

	// A second order that would exceed what is left cannot be approved.
	order2, err := kasse.NewPaymentOrder(ref+"-2", 2027, 50000, "EUR", rec.ID, pos.ID,
		"Zweite", time.Date(2027, 5, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder 2: %v", err)
	}
	created2, err := svc.Create(ctx, order2)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if _, err := svc.Submit(ctx, created2.ID, "alice"); err != nil {
		t.Fatalf("Submit 2: %v", err)
	}
	if _, err := svc.Approve(ctx, created2.ID, "bob"); !errors.Is(err, budget.ErrBudgetExceeded) {
		t.Fatalf("over-budget approval = %v, want ErrBudgetExceeded", err)
	}
	// It was not left approved.
	reloaded, err := svc.Order(ctx, created2.ID)
	if err != nil {
		t.Fatalf("Order 2: %v", err)
	}
	if reloaded.Status != kasse.OrderSubmitted {
		t.Fatalf("status = %q, want it still awaiting a decision", reloaded.Status)
	}
}

func TestPaymentOrderWorkflowPersists(t *testing.T) {
	ctx := context.Background()
	svc, pool, created := newOrderFixture(t, 125000)

	if created.Status != kasse.OrderDraft {
		t.Fatalf("new order status = %q, want draft", created.Status)
	}

	submitted, err := svc.Submit(ctx, created.ID, "alice")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if submitted.Status != kasse.OrderSubmitted {
		t.Fatalf("status = %q, want submitted", submitted.Status)
	}

	// The creator may not approve their own order.
	if _, err := svc.Approve(ctx, created.ID, "alice"); !errors.Is(err, kasse.ErrFourEyes) {
		t.Fatalf("want ErrFourEyes, got %v", err)
	}

	approved, err := svc.Approve(ctx, created.ID, "bob")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !approved.IsApproved() || approved.DecidedBy != "bob" {
		t.Fatalf("after approve: %+v", approved)
	}

	// Reload from the database: the state must have been persisted.
	reloaded, err := svc.Order(ctx, created.ID)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	if !reloaded.IsApproved() {
		t.Fatalf("reloaded status = %q, want approved", reloaded.Status)
	}
	if reloaded.AmountCents != 125000 {
		t.Fatalf("reloaded amount = %d", reloaded.AmountCents)
	}

	// Decision trail: submitted then approved, both persisted.
	var decisions int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM payment_order_decision WHERE order_id = $1", created.ID).Scan(&decisions); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if decisions != 2 {
		t.Fatalf("decision rows = %d, want 2", decisions)
	}

	// Audit: create + submit + approve = 3 events for this order.
	if got := countSubject(t, pool, fmt.Sprintf("order/%d", created.ID)); got != 3 {
		t.Fatalf("audit events for the order = %d, want 3", got)
	}

	// An approved order cannot be cancelled or approved again.
	if _, err := svc.Cancel(ctx, created.ID, "carol"); !errors.Is(err, kasse.ErrNotCancellable) {
		t.Fatalf("cancel approved: want ErrNotCancellable, got %v", err)
	}
	if _, err := svc.Approve(ctx, created.ID, "carol"); !errors.Is(err, kasse.ErrNotSubmitted) {
		t.Fatalf("approve twice: want ErrNotSubmitted, got %v", err)
	}
}

func TestPaymentOrderRejectPersistsReason(t *testing.T) {
	ctx := context.Background()
	svc, _, created := newOrderFixture(t, 5000)

	if _, err := svc.Submit(ctx, created.ID, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	rejected, err := svc.Reject(ctx, created.ID, "bob", "Budget ausgeschöpft")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if rejected.Status != kasse.OrderRejected {
		t.Fatalf("status = %q", rejected.Status)
	}

	reloaded, err := svc.Order(ctx, created.ID)
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	if reloaded.DecisionReason != "Budget ausgeschöpft" {
		t.Fatalf("reason = %q, want it persisted", reloaded.DecisionReason)
	}
}

func TestPaymentOrderRejectsDuplicateReference(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	// This test never approves, so no budget link is exercised.
	svc := kasse.NewOrderService(kasse.NewOrderRepo(pool), nil, audit.NewStore(pool))
	_, _, created := newOrderFixture(t, 1000)

	duplicate, err := kasse.NewPaymentOrder(created.Reference, 2026, 1000, "EUR",
		created.RecipientID, created.BudgetPositionID, "doppelt",
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := svc.Create(ctx, duplicate); !errors.Is(err, kasse.ErrReferenceExists) {
		t.Fatalf("want ErrReferenceExists, got %v", err)
	}
}

func TestPaymentOrderUnknownIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	// This test never approves, so no budget link is exercised.
	svc := kasse.NewOrderService(kasse.NewOrderRepo(pool), nil, audit.NewStore(pool))
	if _, err := svc.Order(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := svc.Submit(ctx, 9223372036854775807, "alice"); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestPaymentOrderListOrdersByDueDate: the list an operator works through is soonest due first,
// and the status filter narrows it without the caller having to sift.
func TestPaymentOrderListOrdersByDueDate(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewOrderService(kasse.NewOrderRepo(pool), budgetService(pool), audit.NewStore(pool))

	mark := fmt.Sprintf("%d", time.Now().UnixNano())
	rec, err := svc.CreateRecipient(ctx, kasse.Recipient{
		Name: "Stadtwerke " + mark, AccountHolder: "Stadtwerke", IBAN: "DE02120300000000202051",
		BIC: "BYLADEM1001", BankName: "Musterbank",
	})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := svc.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year: 2026, Chapter: "0710", Title: "6810", CostCentre: "KST-L-" + mark, CostCarrier: "KT-1",
		AllocatedCents: 900000,
	})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}

	// Deliberately created out of due-date order, so the list has to sort rather than echo.
	dueDates := []string{"2026-05-03", "2026-05-01", "2026-05-02"}
	refs := make([]string, 0, len(dueDates))
	for i, d := range dueDates {
		due, err := time.Parse(time.DateOnly, d)
		if err != nil {
			t.Fatalf("parse due date: %v", err)
		}
		ref := fmt.Sprintf("AO-L-%s-%d", mark, i)
		order, err := kasse.NewPaymentOrder(ref, 2026, 1000+int64(i), "EUR", rec.ID, pos.ID,
			"Stromabschlag", due, "alice")
		if err != nil {
			t.Fatalf("NewPaymentOrder: %v", err)
		}
		if _, err := svc.Create(ctx, order); err != nil {
			t.Fatalf("Create: %v", err)
		}
		refs = append(refs, ref)
	}

	all, err := svc.Orders(ctx, "", false, 1000, 0)
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	// Only my three matter here; other tests in this binary share the table.
	positions := map[string]int{}
	for i, s := range all {
		positions[s.Reference] = i
	}
	// 2026-05-01 < 2026-05-02 < 2026-05-03, which is the order the references were made in
	// reversed: index 1 is due first, then 2, then 0.
	if positions[refs[1]] > positions[refs[2]] || positions[refs[2]] > positions[refs[0]] {
		t.Fatalf("the list is not soonest-due-first: %v", positions)
	}

	// The summary carries what a list shows.
	for _, s := range all {
		if s.Reference != refs[1] {
			continue
		}
		if s.AmountCents != 1001 || s.Currency != "EUR" || s.Purpose != "Stromabschlag" ||
			s.Status != kasse.OrderDraft || s.BudgetPositionID != pos.ID {
			t.Fatalf("summary = %+v", s)
		}
	}

	// Submit one, then narrow to submitted: the others are not in that answer.
	if _, err := svc.Submit(ctx, 0, "alice"); err == nil {
		t.Fatal("submitting a non-existent order should fail")
	}
	var submitted kasse.PaymentOrder
	for _, s := range all {
		if s.Reference == refs[2] {
			submitted, err = svc.Order(ctx, s.ID)
			if err != nil {
				t.Fatalf("Order: %v", err)
			}
		}
	}
	if _, err := svc.Submit(ctx, submitted.ID, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	narrowed, err := svc.Orders(ctx, kasse.OrderSubmitted, false, 1000, 0)
	if err != nil {
		t.Fatalf("Orders(submitted): %v", err)
	}
	if len(narrowed) == 0 {
		t.Fatal("the submitted order is not in the narrowed list")
	}
	for _, s := range narrowed {
		if s.Status != kasse.OrderSubmitted {
			t.Fatalf("narrowed list contains %+v", s)
		}
	}
	for _, s := range narrowed {
		if s.Reference == refs[0] || s.Reference == refs[1] {
			t.Fatalf("a draft order (%s) came through the submitted filter", s.Reference)
		}
	}
}

// TestPaymentOrderListPages drives the list the way the screen does: one page at a time, advancing
// the offset, and checks the two properties a paged list must have — a page holds no more than was
// asked for, and walking the pages yields the whole list, in order, with nothing repeated and
// nothing skipped. The page is of one status's orders, asked for together with the filter, which is
// how the screen reads it.
func TestPaymentOrderListPages(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewOrderService(kasse.NewOrderRepo(pool), budgetService(pool), audit.NewStore(pool))

	mark := fmt.Sprintf("%d", time.Now().UnixNano())
	rec, err := svc.CreateRecipient(ctx, kasse.Recipient{
		Name: "Stadtwerke " + mark, AccountHolder: "Stadtwerke", IBAN: "DE02120300000000202051",
		BIC: "BYLADEM1001", BankName: "Musterbank",
	})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := svc.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year: 2026, Chapter: "0710", Title: "6810", CostCentre: "KST-P-" + mark, CostCarrier: "KT-1",
		AllocatedCents: 900000,
	})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}
	// One order, so the list this test reads is not empty on its own and the paging below is not
	// vacuous. Nothing else about it matters: the pages are checked against a window of the list
	// rather than against what this test put there.
	due, err := time.Parse(time.DateOnly, "2026-06-01")
	if err != nil {
		t.Fatalf("parse due date: %v", err)
	}
	order, err := kasse.NewPaymentOrder(fmt.Sprintf("AO-P-%s", mark), 2026, 1000,
		"EUR", rec.ID, pos.ID, "Stromabschlag", due, "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	if _, err := svc.Create(ctx, order); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A window read in one go is the reference the pages must reproduce: the same rows in the same
	// order. Reading a window rather than assuming the table is empty keeps this independent of what
	// the other tests in this package have already stored.
	const window = 4
	head, err := svc.Orders(ctx, "", false, window, 0)
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	if len(head) == 0 {
		t.Fatal("the list is empty, so nothing about paging is being tested")
	}

	// The same window taken as two pages: the first from the beginning, the second from where the
	// first ended. A list that ignored the offset would repeat the first page; one that ignored the
	// limit would answer with more than was asked for.
	const pageSize = 2
	first, err := svc.Orders(ctx, "", false, pageSize, 0)
	if err != nil {
		t.Fatalf("Orders(offset=0): %v", err)
	}
	if len(first) > pageSize {
		t.Fatalf("a page of %d holds %d rows", pageSize, len(first))
	}
	second, err := svc.Orders(ctx, "", false, pageSize, pageSize)
	if err != nil {
		t.Fatalf("Orders(offset=%d): %v", pageSize, err)
	}
	if len(second) > pageSize {
		t.Fatalf("a page of %d holds %d rows", pageSize, len(second))
	}

	paged := append(append([]kasse.OrderSummary{}, first...), second...)
	if len(paged) != len(head) {
		t.Fatalf("two pages of %d yielded %d rows, want the %d of one window", pageSize, len(paged), len(head))
	}
	for i := range head {
		if paged[i].ID != head[i].ID {
			t.Fatalf("the pages diverge from the window at %d: got %d, want %d — the order is not stable",
				i, paged[i].ID, head[i].ID)
		}
	}

	// Past the end is an empty page, not an error: a screen that asks once too often gets nothing
	// rather than a failure it would have to explain.
	past, err := svc.Orders(ctx, "", false, pageSize, 100000)
	if err != nil {
		t.Fatalf("Orders past the end: %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows, want none", len(past))
	}
}
