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
