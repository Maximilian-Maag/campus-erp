package procurement_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// twoLines is an order of two lines; with 19 % VAT the gross is 13090.
func twoLines() []procurement.Line {
	return []procurement.Line{
		{LineNo: 1, Description: "Ware", QuantityMilli: 1000, UnitPriceCents: 10000, NetCents: 10000, VatCategory: "S", VatRateBP: 1900},
		{LineNo: 2, Description: "Versand", QuantityMilli: 2000, UnitPriceCents: 500, NetCents: 1000, VatCategory: "S", VatRateBP: 1900},
	}
}

func receiptDay(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }

// TestGoodsReceiptConvertsPartially is the acceptance test of the slice: a delivery converts the
// value of what arrived from a reservation into a consumption without changing the position's
// available Mittel, the order stays open until every line has arrived, and the receipts of the
// order add up to its gross exactly.
func TestGoodsReceiptConvertsPartially(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "WE-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	order, err := procSvc.Create(ctx, "PO-WE-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", twoLines(), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if order.GrossCents != 13090 {
		t.Fatalf("gross = %d, want 13090", order.GrossCents)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "alice"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Before any delivery: nothing arrived, so nothing is open at the receipt read either, and
	// the whole order is open.
	state, err := procSvc.ReceiptState(ctx, order.ID)
	if err != nil {
		t.Fatalf("ReceiptState: %v", err)
	}
	if len(state.Receipts) != 0 || len(state.Progress) != 2 || state.Complete {
		t.Fatalf("state before any receipt = %+v", state)
	}
	if p := state.Progress[0]; p.ReceivedMilli != 0 || p.OpenMilli() != 1000 {
		t.Fatalf("progress before = %+v", p)
	}

	// A first, partial delivery: half of the first line.
	result, err := procSvc.RecordReceipt(ctx, order.ID, "LS-1-"+suffix(t), receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 500}}, "carol")
	if err != nil {
		t.Fatalf("RecordReceipt: %v", err)
	}
	if result.Receipt.ReceivedCents != 5950 || result.Receipt.ReceivedBy != "carol" {
		t.Fatalf("receipt = %+v", result.Receipt)
	}
	if result.State.Complete || result.State.OrderID != order.ID {
		t.Fatalf("state after the first receipt = %+v", result.State)
	}
	if p := result.State.Progress[0]; p.ReceivedMilli != 500 || p.OpenMilli() != 500 || p.Complete() {
		t.Fatalf("first line after half a delivery = %+v", p)
	}
	if len(result.State.Receipts) != 1 || result.State.Receipts[0].Reference != result.Receipt.Reference {
		t.Fatalf("receipts after the first delivery = %+v", result.State.Receipts)
	}
	if len(result.State.Receipts[0].Lines) != 1 || result.State.Receipts[0].Lines[0].QuantityMilli != 500 {
		t.Fatalf("stored receipt lines = %+v", result.State.Receipts[0].Lines)
	}

	// The order is still approved (not everything arrived) and the conversion consumed exactly
	// what arrived: the reservation fell, the actual rose, the available Mittel are unchanged.
	still, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if still.Status != procurement.StatusApproved {
		t.Fatalf("status after a partial delivery = %s, want approved", still.Status)
	}
	bal, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 13090-5950 || bal.ActualCents != 5950 {
		t.Fatalf("balance after the partial delivery = committed %d / actual %d", bal.CommittedCents, bal.ActualCents)
	}
	if bal.AvailableCents() != 100000-13090 {
		t.Fatalf("available = %d, want the position less the order's gross", bal.AvailableCents())
	}

	// The rest arrives: the order is complete and the remaining commitment is consumed, so the
	// receipts add up to the gross.
	final, err := procSvc.RecordReceipt(ctx, order.ID, "LS-2-"+suffix(t), receiptDay(25),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 500}, {LineNo: 2, QuantityMilli: 2000}}, "carol")
	if err != nil {
		t.Fatalf("RecordReceipt (final): %v", err)
	}
	if !final.State.Complete {
		t.Fatalf("state after everything arrived = %+v", final.State)
	}
	if got := final.Receipt.ReceivedCents + 5950; got != 13090 {
		t.Fatalf("the receipts convert %d, want the order's gross 13090", got)
	}
	complete, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if complete.Status != procurement.StatusReceived {
		t.Fatalf("status after everything arrived = %s, want received", complete.Status)
	}
	bal, err = budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 0 || bal.ActualCents != 13090 || bal.AvailableCents() != 100000-13090 {
		t.Fatalf("final balance = committed %d / actual %d / available %d",
			bal.CommittedCents, bal.ActualCents, bal.AvailableCents())
	}
	for _, p := range final.State.Progress {
		if !p.Complete() || p.OpenMilli() != 0 {
			t.Fatalf("progress after everything arrived = %+v", p)
		}
	}

	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestGoodsReceiptRefusals: every refusal leaves the order and the budget exactly as they were,
// and records no receipt.
func TestGoodsReceiptRefusals(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "WR-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	order, err := procSvc.Create(ctx, "PO-WR-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", twoLines(), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A draft never committed anything, so a delivery against it is refused.
	if _, err := procSvc.RecordReceipt(ctx, order.ID, "LS-D", receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, "carol"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("delivery on a draft = %v, want ErrBadTransition", err)
	}
	// An unknown order, and a blank actor.
	if _, err := procSvc.RecordReceipt(ctx, 9223372036854775807, "LS-X", receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, "carol"); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("unknown order = %v, want ErrOrderNotFound", err)
	}
	if _, err := procSvc.RecordReceipt(ctx, order.ID, "LS-Y", receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, "  "); !errors.Is(err, procurement.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
	if _, err := procSvc.ReceiptState(ctx, 9223372036854775807); !errors.Is(err, procurement.ErrOrderNotFound) {
		t.Fatalf("state of an unknown order = %v, want ErrOrderNotFound", err)
	}

	if _, err := procSvc.Approve(ctx, order.ID, "alice"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	before, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	reference := "LS-R-" + suffix(t)
	if _, err := procSvc.RecordReceipt(ctx, order.ID, reference, receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 500}}, "carol"); err != nil {
		t.Fatalf("first receipt: %v", err)
	}
	after, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}

	// More of a line than was ordered is refused rather than clamped.
	if _, err := procSvc.RecordReceipt(ctx, order.ID, "LS-OVER-"+suffix(t), receiptDay(21),
		[]procurement.ReceiptLine{{LineNo: 1, QuantityMilli: 501}}, "carol"); !errors.Is(err, procurement.ErrOverReceipt) {
		t.Fatalf("over-receipt = %v, want ErrOverReceipt", err)
	}
	// A line the order does not carry.
	if _, err := procSvc.RecordReceipt(ctx, order.ID, "LS-GHOST-"+suffix(t), receiptDay(21),
		[]procurement.ReceiptLine{{LineNo: 9, QuantityMilli: 1}}, "carol"); !errors.Is(err, procurement.ErrReceiptLineUnknown) {
		t.Fatalf("unknown line = %v, want ErrReceiptLineUnknown", err)
	}
	// No lines at all.
	if _, err := procSvc.RecordReceipt(ctx, order.ID, "LS-EMPTY-"+suffix(t), receiptDay(21), nil, "carol"); !errors.Is(err, procurement.ErrNoReceiptLines) {
		t.Fatalf("empty receipt = %v, want ErrNoReceiptLines", err)
	}
	// The same delivery note twice.
	if _, err := procSvc.RecordReceipt(ctx, order.ID, reference, receiptDay(21),
		[]procurement.ReceiptLine{{LineNo: 2, QuantityMilli: 1}}, "carol"); !errors.Is(err, procurement.ErrReceiptExists) {
		t.Fatalf("duplicate reference = %v, want ErrReceiptExists", err)
	}

	// None of the refusals moved anything, and each is recorded nowhere.
	still, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if still.CommittedCents != after.CommittedCents || still.ActualCents != after.ActualCents {
		t.Fatalf("a refused receipt moved the budget: %+v -> %+v", after, still)
	}
	if after.ActualCents == before.ActualCents {
		t.Fatalf("the first receipt did not convert anything: %+v -> %+v", before, after)
	}
	state, err := procSvc.ReceiptState(ctx, order.ID)
	if err != nil {
		t.Fatalf("ReceiptState: %v", err)
	}
	if len(state.Receipts) != 1 {
		t.Fatalf("receipts recorded = %d, want only the accepted one", len(state.Receipts))
	}
	current, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if current.Status != procurement.StatusApproved {
		t.Fatalf("status = %s, want approved", current.Status)
	}
}

// TestInvoiceMatchAfterAPartialReceipt: an invoice that arrives after a delivery converts what is
// left of the commitment, so the order is consumed exactly once rather than twice.
func TestInvoiceMatchAfterAPartialReceipt(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "WN-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	order, err := procSvc.Create(ctx, "PO-WN-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", twoLines(), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "alice"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	// A quarter of the order arrives first (line 2 half in: 1000 of 2000 milli).
	partial, err := procSvc.RecordReceipt(ctx, order.ID, "LS-N-"+suffix(t), receiptDay(20),
		[]procurement.ReceiptLine{{LineNo: 2, QuantityMilli: 1000}}, "carol")
	if err != nil {
		t.Fatalf("RecordReceipt: %v", err)
	}
	// Half of line 2 is worth net 500 plus 19 % = 595.
	converted := partial.Receipt.ReceivedCents
	if converted != 595 {
		t.Fatalf("the partial delivery converted %d, want 595", converted)
	}
	if partial.State.Complete {
		t.Fatal("a half line cannot complete the order")
	}
	if p := partial.State.Progress[1]; p.ReceivedMilli != 1000 || p.OpenMilli() != 1000 {
		t.Fatalf("second line after half a delivery = %+v", p)
	}

	// The invoice then matches the order in full, but only the remainder is left to convert.
	matched, err := procSvc.MatchReceipt(ctx, order.Number, order.GrossCents, "RE-N-"+suffix(t), "carol")
	if err != nil {
		t.Fatalf("MatchReceipt: %v", err)
	}
	if matched.Status != procurement.StatusReceived {
		t.Fatalf("status after the match = %s, want received", matched.Status)
	}
	bal, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 0 {
		t.Fatalf("committed after the match = %d, want 0", bal.CommittedCents)
	}
	if bal.ActualCents != order.GrossCents {
		t.Fatalf("actual = %d, want the order's gross %d (the delivery plus the invoice)", bal.ActualCents, order.GrossCents)
	}
	if bal.AvailableCents() != 100000-order.GrossCents {
		t.Fatalf("available = %d, want the position less the order's gross", bal.AvailableCents())
	}

	// An order whose commitment is entirely converted cannot be matched again: it is no longer
	// approved, and there is nothing left to convert.
	if _, err := procSvc.MatchReceipt(ctx, order.Number, order.GrossCents, "RE-N2", "carol"); !errors.Is(err, procurement.ErrBadTransition) {
		t.Fatalf("second match = %v, want ErrBadTransition", err)
	}
	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
