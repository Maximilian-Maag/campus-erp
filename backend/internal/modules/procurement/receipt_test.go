package procurement

import (
	"errors"
	"testing"
	"time"
)

func receiptOrder() Order {
	return Order{ID: 7, Number: "PO-1", GrossCents: 13090, Lines: lines()}
}

func receiptDay() time.Time { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) }

// A partial receipt is worth the price of what arrived: the line's net at its unit price plus
// the line's own VAT.
func TestReceiptLineValue(t *testing.T) {
	order := receiptOrder()
	if got := ReceiptLineValue(order.Lines[0], 1000); got != 11900 {
		t.Fatalf("the whole first line = %d, want 11900", got)
	}
	if got := ReceiptLineValue(order.Lines[0], 500); got != 5950 {
		t.Fatalf("half the first line = %d, want 5950", got)
	}
	if got := ReceiptLineValue(order.Lines[1], 2000); got != 1190 {
		t.Fatalf("the second line = %d, want 1190", got)
	}
}

// The receipts of an order add up to its gross exactly: the receipt that completes the order
// converts the remaining commitment rather than its own computed value, which is what absorbs
// the difference between rounding a group's VAT once and a part's VAT per part.
func TestReceiptsAddUpToTheOrderGross(t *testing.T) {
	order := receiptOrder()
	// Half the first line, then everything else.
	_, first, complete, err := NewReceipt(order, "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 500}}, nil, 0)
	if err != nil {
		t.Fatalf("first receipt: %v", err)
	}
	if complete {
		t.Fatal("a half line cannot complete the order")
	}
	if first != 5950 {
		t.Fatalf("first receipt value = %d, want 5950", first)
	}
	_, second, complete, err := NewReceipt(order, "LS-2", receiptDay(),
		[]ReceiptLine{{LineNo: 1, QuantityMilli: 500}, {LineNo: 2, QuantityMilli: 2000}},
		map[int]int64{1: 500}, first)
	if err != nil {
		t.Fatalf("second receipt: %v", err)
	}
	if !complete {
		t.Fatal("everything has arrived; the order must be complete")
	}
	if second != order.GrossCents-first {
		t.Fatalf("the completing receipt converts %d, want the remainder %d", second, order.GrossCents-first)
	}
	if first+second != order.GrossCents {
		t.Fatalf("receipts add up to %d, want the order's gross %d", first+second, order.GrossCents)
	}
}

// A receipt that completes the order converts the remainder even when its own computed value is
// larger, and one that does not complete it never converts more than remains.
func TestReceiptValueNeverExceedsTheRemainingCommitment(t *testing.T) {
	order := receiptOrder()
	// The whole order in one receipt: computed == gross, so the value is the gross.
	_, whole, complete, err := NewReceipt(order, "LS-1", receiptDay(),
		[]ReceiptLine{{LineNo: 1, QuantityMilli: 1000}, {LineNo: 2, QuantityMilli: 2000}}, nil, 0)
	if err != nil {
		t.Fatalf("whole receipt: %v", err)
	}
	if !complete || whole != order.GrossCents {
		t.Fatalf("whole receipt = %d complete=%t", whole, complete)
	}
	// Something has already converted more than the order's gross (an inconsistent state): the
	// remainder is zero, so the receipt converts nothing rather than a negative amount.
	if got := remainingCommitment(100, 150); got != 0 {
		t.Fatalf("remainingCommitment(100,150) = %d, want 0", got)
	}
	if got := remainingCommitment(100, 100); got != 0 {
		t.Fatalf("remainingCommitment(100,100) = %d, want 0", got)
	}
	if got := remainingCommitment(100, 40); got != 60 {
		t.Fatalf("remainingCommitment(100,40) = %d, want 60", got)
	}
	// Already fully converted: a further receipt of a line that is still open converts the
	// remainder (zero here), never less than zero.
	_, clamped, _, err := NewReceipt(order, "LS-2", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 500}},
		map[int]int64{1: 500}, order.GrossCents)
	if err != nil {
		t.Fatalf("clamped receipt: %v", err)
	}
	if clamped != 0 {
		t.Fatalf("clamped receipt converts %d, want 0", clamped)
	}
}

func TestNewReceiptRefusals(t *testing.T) {
	order := receiptOrder()
	cases := []struct {
		name       string
		reference  string
		receivedOn time.Time
		lines      []ReceiptLine
		already    map[int]int64
		want       error
	}{
		{"blank reference", "  ", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, nil, ErrReceiptReferenceRequired},
		{"no date", "LS-1", time.Time{}, []ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, nil, ErrReceiptDateRequired},
		{"no lines", "LS-1", receiptDay(), nil, nil, ErrNoReceiptLines},
		{"zero quantity", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 0}}, nil, ErrReceiptQuantity},
		{"negative quantity", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: -5}}, nil, ErrReceiptQuantity},
		{"unknown line", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 9, QuantityMilli: 1}}, nil, ErrReceiptLineUnknown},
		{"duplicate line", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 1}, {LineNo: 1, QuantityMilli: 1}}, nil, ErrReceiptLineDuplicate},
		{"over receipt", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 1001}}, nil, ErrOverReceipt},
		{"over the earlier receipt", "LS-1", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 600}}, map[int]int64{1: 500}, ErrOverReceipt},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, _, err := NewReceipt(order, c.reference, c.receivedOn, c.lines, c.already, 0)
			if !errors.Is(err, c.want) {
				t.Fatalf("NewReceipt error = %v, want %v", err, c.want)
			}
		})
	}
	// A refusal writes nothing: the receipt must come back empty.
	receipt, value, complete, err := NewReceipt(order, "  ", receiptDay(), []ReceiptLine{{LineNo: 1, QuantityMilli: 1}}, nil, 0)
	if !errors.Is(err, ErrReceiptReferenceRequired) {
		t.Fatalf("error = %v", err)
	}
	if receipt.Reference != "" || receipt.OrderID != 0 || len(receipt.Lines) != 0 || value != 0 || complete {
		t.Fatalf("a refused receipt = %+v value=%d complete=%t", receipt, value, complete)
	}
}

// A receipt takes the order's identity, its own reference and date, and trims the reference.
func TestNewReceiptBuildsTheReceipt(t *testing.T) {
	order := receiptOrder()
	lines := []ReceiptLine{{LineNo: 1, QuantityMilli: 1000}, {LineNo: 2, QuantityMilli: 500}}
	receipt, value, complete, err := NewReceipt(order, "  LS-7  ", receiptDay(), lines, nil, 0)
	if err != nil {
		t.Fatalf("NewReceipt: %v", err)
	}
	if receipt.OrderID != order.ID || receipt.Reference != "LS-7" || !receipt.ReceivedOn.Equal(receiptDay()) {
		t.Fatalf("receipt = %+v", receipt)
	}
	if len(receipt.Lines) != 2 || receipt.Lines[1].QuantityMilli != 500 {
		t.Fatalf("receipt lines = %+v", receipt.Lines)
	}
	// line 2 is only a quarter in, so the order is not complete.
	if complete {
		t.Fatal("a partly received line cannot complete the order")
	}
	if value != 11900+298 {
		t.Fatalf("value = %d, want 12198", value)
	}
}

func TestIsFullyReceived(t *testing.T) {
	byNo := lineByNo(receiptOrder())
	if IsFullyReceived(nil, nil) {
		t.Fatal("an order with no lines is never fully received")
	}
	if IsFullyReceived(byNo, nil) {
		t.Fatal("nothing received is not fully received")
	}
	if IsFullyReceived(byNo, map[int]int64{1: 1000}) {
		t.Fatal("one line of two is not fully received")
	}
	if IsFullyReceived(byNo, map[int]int64{1: 1000, 2: 1999}) {
		t.Fatal("one unit short is not fully received")
	}
	if !IsFullyReceived(byNo, map[int]int64{1: 1000, 2: 2000}) {
		t.Fatal("every line in full is fully received")
	}
	// More than ordered still counts as received (the domain refuses over-receipt, but a stored
	// state must not read as open).
	if !IsFullyReceived(byNo, map[int]int64{1: 1200, 2: 2000}) {
		t.Fatal("over-received is received")
	}
}

func TestProgressAndOpenQuantities(t *testing.T) {
	order := receiptOrder()
	progress := Progress(order, map[int]int64{1: 400})
	if len(progress) != 2 {
		t.Fatalf("progress = %+v", progress)
	}
	if p := progress[0]; p.LineNo != 1 || p.Description != "Ware" || p.OrderedMilli != 1000 || p.ReceivedMilli != 400 {
		t.Fatalf("first progress = %+v", p)
	}
	if progress[0].OpenMilli() != 600 || progress[0].Complete() {
		t.Fatalf("open = %d complete=%t", progress[0].OpenMilli(), progress[0].Complete())
	}
	// A line nothing arrived of is wholly open.
	if p := progress[1]; p.ReceivedMilli != 0 || p.OpenMilli() != 2000 || p.Complete() {
		t.Fatalf("second progress = %+v open=%d", p, p.OpenMilli())
	}
	// A complete line is not open, and an over-received one does not report a negative quantity.
	for _, p := range []LineProgress{
		{OrderedMilli: 1000, ReceivedMilli: 1000},
		{OrderedMilli: 1000, ReceivedMilli: 1200},
	} {
		if p.OpenMilli() != 0 || !p.Complete() {
			t.Fatalf("progress %+v open=%d complete=%t", p, p.OpenMilli(), p.Complete())
		}
	}
}
