package einvoice_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// matchServices wires the three modules over one pool: an invoice is matched against a purchase
// order, which converts its commitment on the budget position.
func matchServices(t *testing.T) (*einvoice.InvoiceService, *procurement.OrderService, *budget.PositionService) {
	t.Helper()
	pool := testPool(t)
	store := audit.NewStore(pool)
	budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
	procSvc := procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store)
	return einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), procSvc, store), procSvc, budgetSvc
}

// TestMatchInvoiceToPurchaseOrder runs the two-way match end to end: an approved order on a
// funded position, an inbound invoice whose buyer reference is that order's number and whose
// gross is the same, and the match converting the commitment into an actual.
func TestMatchInvoiceToPurchaseOrder(t *testing.T) {
	ctx := context.Background()
	inSvc, procSvc, budgetSvc := matchServices(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "0710", "6810", "Beschaffung", 1000000, "carol")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	number := "PO-" + uniqueSuffix(t)
	line := procurement.Line{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000,
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}
	order, err := procSvc.Create(ctx, number, "Lieferant GmbH", pos.ID, "EUR", []procurement.Line{line}, "carol")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	order, err = procSvc.Approve(ctx, order.ID, "dave")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if order.GrossCents != 35700 {
		t.Fatalf("order gross = %d, want 35700", order.GrossCents)
	}
	// Approving bound the Mittel.
	bound, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bound.CommittedCents != 35700 {
		t.Fatalf("committed = %d, want the order's 35700", bound.CommittedCents)
	}

	// The supplier's invoice: the same line, and the order number as its buyer reference.
	inv := sampleInvoice(t)
	inv.Number = "RE-" + uniqueSuffix(t)
	inv.BuyerReference = number
	inv.Lines = []core.Line{{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000, UnitCode: "C62",
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}}
	doc, err := core.GenerateXRechnung(inv)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	saved, err := inSvc.ImportReceived(ctx, doc, "carol")
	if err != nil {
		t.Fatalf("ImportReceived: %v", err)
	}

	matched, err := inSvc.Match(ctx, saved.ID, "carol")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if matched.ID != saved.ID {
		t.Fatalf("Match returned %d, want %d", matched.ID, saved.ID)
	}

	// The order is received, and the commitment became an actual: the conversion must not
	// double-count, so committed drops to zero and the actual carries the amount.
	got, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get order: %v", err)
	}
	if string(got.Status) != "received" {
		t.Fatalf("order status = %s, want received", got.Status)
	}
	after, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if after.CommittedCents != 0 || after.ActualCents != 35700 {
		t.Fatalf("balance = committed %d / actual %d, want 0 / 35700", after.CommittedCents, after.ActualCents)
	}
	if after.AvailableCents() != 1000000-35700 {
		t.Fatalf("available = %d, want the position less the actual", after.AvailableCents())
	}
}

// TestMatchRefusesAWrongAmount: a two-way match is only as good as the amount check, and a
// mismatch must leave both the order and the budget exactly as they were.
func TestMatchRefusesAWrongAmount(t *testing.T) {
	ctx := context.Background()
	inSvc, procSvc, budgetSvc := matchServices(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "0711", "6810", "Beschaffung", 1000000, "carol")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	number := "PO-" + uniqueSuffix(t)
	line := procurement.Line{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000,
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}
	order, err := procSvc.Create(ctx, number, "Lieferant GmbH", pos.ID, "EUR", []procurement.Line{line}, "carol")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "dave"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// One cent more than the order.
	inv := sampleInvoice(t)
	inv.Number = "RE-" + uniqueSuffix(t)
	inv.BuyerReference = number
	inv.Lines = []core.Line{{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000, UnitCode: "C62",
		UnitPriceCents: 30001, NetCents: 30001, VatCategory: "S", VatRateBP: 1900}}
	doc, err := core.GenerateXRechnung(inv)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	saved, err := inSvc.ImportReceived(ctx, doc, "carol")
	if err != nil {
		t.Fatalf("ImportReceived: %v", err)
	}

	if _, err := inSvc.Match(ctx, saved.ID, "carol"); !errors.Is(err, procurement.ErrMatchAmountMismatch) {
		t.Fatalf("Match = %v, want ErrMatchAmountMismatch", err)
	}
	// Untouched: still approved, and the commitment still stands.
	got, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Status) != "approved" {
		t.Fatalf("order status = %s, want approved", got.Status)
	}
	after, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if after.CommittedCents != 35700 || after.ActualCents != 0 {
		t.Fatalf("balance = committed %d / actual %d, want 35700 / 0", after.CommittedCents, after.ActualCents)
	}
}

// TestMatchRefusesAnUnknownOrder: an invoice naming an order that does not exist.
func TestMatchRefusesAnUnknownOrder(t *testing.T) {
	ctx := context.Background()
	inSvc, _, _ := matchServices(t)

	inv := sampleInvoice(t)
	inv.Number = "RE-" + uniqueSuffix(t)
	inv.BuyerReference = "PO-does-not-exist-" + uniqueSuffix(t)
	doc, err := core.GenerateXRechnung(inv)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	saved, err := inSvc.ImportReceived(ctx, doc, "carol")
	if err != nil {
		t.Fatalf("ImportReceived: %v", err)
	}
	if _, err := inSvc.Match(ctx, saved.ID, "carol"); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("Match = %v, want ErrOrderNotFound", err)
	}
}

// TestMatchIsRecordedOnTheAuditChain: matching twice fails (the order is already received) and
// the chain still verifies.
func TestMatchIsRecordedOnTheAuditChain(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
	procSvc := procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store)
	inSvc := einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), procSvc, store)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "0712", "6810", "Beschaffung", 1000000, "carol")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	number := "PO-" + uniqueSuffix(t)
	line := procurement.Line{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000,
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}
	order, err := procSvc.Create(ctx, number, "Lieferant GmbH", pos.ID, "EUR", []procurement.Line{line}, "carol")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "dave"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	inv := sampleInvoice(t)
	inv.Number = "RE-" + uniqueSuffix(t)
	inv.BuyerReference = number
	inv.Lines = []core.Line{{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000, UnitCode: "C62",
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}}
	doc, err := core.GenerateXRechnung(inv)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	saved, err := inSvc.ImportReceived(ctx, doc, "carol")
	if err != nil {
		t.Fatalf("ImportReceived: %v", err)
	}
	if _, err := inSvc.Match(ctx, saved.ID, "carol"); err != nil {
		t.Fatalf("Match: %v", err)
	}
	// A second match cannot succeed: the order left the approved state.
	if _, err := inSvc.Match(ctx, saved.ID, "carol"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("second match = %v, want ErrBadTransition", err)
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
