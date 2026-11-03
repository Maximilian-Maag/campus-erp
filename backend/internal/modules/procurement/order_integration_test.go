package procurement_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.Pool(t)
}

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func suffix(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000000")
}

func setup(t *testing.T) (*procurement.OrderService, *budget.PositionService) {
	t.Helper()
	pool := testPool(t)
	store := audit.NewStore(pool)
	budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
	procSvc := procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store)
	return procSvc, budgetSvc
}

func oneLine(net, rateBP int64) []procurement.Line {
	return []procurement.Line{{
		LineNo: 1, Description: "Ware", QuantityMilli: 1000, UnitPriceCents: net,
		NetCents: net, VatCategory: "S", VatRateBP: rateBP,
	}}
}

// TestProcurementCommitsBudget is the acceptance test: approving an order reduces
// the budget position's available amount; cancelling restores it; over-budget
// approval is rejected.
func TestProcurementCommitsBudget(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "T-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}

	// net 10000 + 19% = 11900 gross.
	order, err := procSvc.Create(ctx, "PO-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(10000, 1900), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if order.GrossCents != 11900 || order.Status != procurement.StatusDraft {
		t.Fatalf("order = %+v", order)
	}

	// Draft does not touch the budget.
	bal, _ := budgetSvc.Balance(ctx, pos.ID)
	if bal.AvailableCents() != 100000 {
		t.Fatalf("available after draft = %d", bal.AvailableCents())
	}

	// Approve commits the gross.
	approved, err := procSvc.Approve(ctx, order.ID, "alice")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.Status != procurement.StatusApproved {
		t.Fatalf("status = %s", approved.Status)
	}
	bal, _ = budgetSvc.Balance(ctx, pos.ID)
	if bal.CommittedCents != 11900 || bal.AvailableCents() != 88100 {
		t.Fatalf("budget after approve = %+v (available %d)", bal, bal.AvailableCents())
	}

	// An order exceeding the available budget cannot be approved.
	big, err := procSvc.Create(ctx, "PO-BIG-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(100000, 1900), "alice")
	if err != nil {
		t.Fatalf("Create big: %v", err)
	}
	if _, err := procSvc.Approve(ctx, big.ID, "alice"); !errors.Is(err, budget.ErrBudgetExceeded) {
		t.Fatalf("over-budget approve = %v, want ErrBudgetExceeded", err)
	}

	// Receive.
	rec, err := procSvc.Receive(ctx, order.ID, "alice")
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if rec.Status != procurement.StatusReceived {
		t.Fatalf("status = %s", rec.Status)
	}

	// Cancel an approved order releases the commitment.
	other, err := procSvc.Create(ctx, "PO-C-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(5000, 1900), "alice")
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}
	if _, err := procSvc.Approve(ctx, other.ID, "alice"); err != nil {
		t.Fatalf("Approve other: %v", err)
	}
	bal, _ = budgetSvc.Balance(ctx, pos.ID)
	committedBefore := bal.CommittedCents
	if _, err := procSvc.Cancel(ctx, other.ID, "alice"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	bal, _ = budgetSvc.Balance(ctx, pos.ID)
	if bal.CommittedCents != committedBefore-5950 {
		t.Fatalf("committed after cancel = %d, want %d", bal.CommittedCents, committedBefore-5950)
	}

	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestProcurementTransitionsAndErrors(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "T2-"+suffix(t), "", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}

	if _, err := procSvc.Create(ctx, "PO-1", "L", pos.ID, "EUR", oneLine(100, 1900), "  "); !errors.Is(err, procurement.ErrActorRequired) {
		t.Fatalf("create actor: %v", err)
	}

	order, err := procSvc.Create(ctx, "PO-T-"+suffix(t), "L", pos.ID, "EUR", oneLine(100, 1900), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Duplicate number.
	if _, err := procSvc.Create(ctx, order.Number, "L", pos.ID, "EUR", oneLine(100, 1900), "alice"); !errors.Is(err, procurement.ErrOrderExists) {
		t.Fatalf("duplicate: %v", err)
	}
	// Receiving a draft is not allowed.
	if _, err := procSvc.Receive(ctx, order.ID, "alice"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("receive draft: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "alice"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// Approving twice is not allowed.
	if _, err := procSvc.Approve(ctx, order.ID, "alice"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("approve twice: %v", err)
	}
	// Unknown order.
	if _, err := procSvc.Get(ctx, 9223372036854775807); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("unknown get: %v", err)
	}
	if _, err := procSvc.Approve(ctx, 9223372036854775807, "alice"); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("unknown approve: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "  "); !errors.Is(err, procurement.ErrActorRequired) {
		t.Fatalf("approve actor: %v", err)
	}
	if _, err := procSvc.Cancel(ctx, order.ID, "  "); !errors.Is(err, procurement.ErrActorRequired) {
		t.Fatalf("cancel actor: %v", err)
	}
	// Receive then cancel is not allowed.
	if _, err := procSvc.Receive(ctx, order.ID, "alice"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if _, err := procSvc.Cancel(ctx, order.ID, "alice"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("cancel received: %v", err)
	}
	// Cancelling an unknown order.
	if _, err := procSvc.Cancel(ctx, 9223372036854775807, "alice"); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("cancel unknown: %v", err)
	}
}

// TestProcurementMatchReceipt: the two-way match finds the order by number, checks the invoice's
// gross against the order's, and converts the commitment into an actual rather than counting it
// twice. Every refusal leaves the order and the budget as they were.
func TestProcurementMatchReceipt(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "M-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}

	// A matched order: commitment out, actual in, available unchanged by the conversion.
	order, err := procSvc.Create(ctx, "PO-M-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(10000, 1900), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "dave"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	matched, err := procSvc.MatchReceipt(ctx, order.Number, 11900, "RE:1", "carol")
	if err != nil {
		t.Fatalf("MatchReceipt: %v", err)
	}
	if matched.Status != procurement.StatusReceived {
		t.Fatalf("status = %s, want received", matched.Status)
	}
	bal, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 0 || bal.ActualCents != 11900 {
		t.Fatalf("balance = committed %d / actual %d, want 0 / 11900", bal.CommittedCents, bal.ActualCents)
	}
	if bal.AvailableCents() != 100000-11900 {
		t.Fatalf("available = %d, want the position less the actual", bal.AvailableCents())
	}

	// An invoice that does not add up to the order is refused, and nothing moves.
	other, err := procSvc.Create(ctx, "PO-MM-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(5000, 1900), "alice")
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}
	if _, err := procSvc.Approve(ctx, other.ID, "dave"); err != nil {
		t.Fatalf("Approve other: %v", err)
	}
	before, _ := budgetSvc.Balance(ctx, pos.ID)
	if _, err := procSvc.MatchReceipt(ctx, other.Number, 5951, "RE:2", "carol"); !errors.Is(err, procurement.ErrMatchAmountMismatch) {
		t.Fatalf("mismatch = %v, want ErrMatchAmountMismatch", err)
	}
	still, err := procSvc.Get(ctx, other.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if still.Status != procurement.StatusApproved {
		t.Fatalf("status after mismatch = %s, want approved", still.Status)
	}
	if after, _ := budgetSvc.Balance(ctx, pos.ID); after.CommittedCents != before.CommittedCents {
		t.Fatalf("committed moved on a refused match: %d -> %d", before.CommittedCents, after.CommittedCents)
	}

	// A draft cannot be matched (it never committed anything).
	draft, err := procSvc.Create(ctx, "PO-D-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(100, 1900), "alice")
	if err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	if _, err := procSvc.MatchReceipt(ctx, draft.Number, 119, "RE:3", "carol"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("match draft = %v, want ErrBadTransition", err)
	}
	// An unknown order number, and a blank actor.
	if _, err := procSvc.MatchReceipt(ctx, "PO-nope-"+suffix(t), 119, "RE:4", "carol"); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("unknown order = %v, want ErrOrderNotFound", err)
	}
	if _, err := procSvc.MatchReceipt(ctx, order.Number, 11900, "RE:5", "  "); !errors.Is(err, procurement.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}

	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
