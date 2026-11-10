package procurement

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Goods receipts (Wareneingang): what actually arrived, line by line (docs/12).
//
// The whole-order match (MatchReceipt) can only convert an order in full, because an inbound
// invoice states one amount for one order. A delivery does not: it arrives line by line, in
// parts, and often before any invoice. A receipt therefore records the *quantity* received per
// order line, and a partial receipt becomes a fact of its own value — the price of what arrived.
//
// A receipt never changes what the order committed: approving reserved the gross, and a receipt
// converts the value of the goods that arrived from a reservation into a consumption (`release`
// plus `actual` of the same amount), so the position's available Mittel are unchanged by the
// conversion. The order becomes `received` once every line has arrived in full.
//
// The receipts of an order always add up to the order's gross **to the cent**: a partial receipt
// is valued as the price of its own quantities (net at the line price plus the line's VAT), and
// the receipt that completes the order converts whatever is left of the commitment rather than
// its own computed value. Rounding a group's VAT once (the order) and a part's VAT per part
// cannot agree in general, so the residual belongs to the last part — that is what makes the sum
// exact instead of nearly exact.

// Errors of the goods receipt.
var (
	// ErrReceiptReferenceRequired is returned when a receipt names no delivery note.
	ErrReceiptReferenceRequired = errors.New("procurement: a receipt reference is required")
	// ErrReceiptDateRequired is returned when a receipt carries no date of receipt.
	ErrReceiptDateRequired = errors.New("procurement: a receipt date is required")
	// ErrNoReceiptLines is returned when a receipt carries no line.
	ErrNoReceiptLines = errors.New("procurement: a receipt needs at least one line")
	// ErrReceiptQuantity is returned when a receipt line's quantity is not positive.
	ErrReceiptQuantity = errors.New("procurement: a received quantity must be positive")
	// ErrReceiptLineUnknown is returned when a receipt names a line the order does not carry.
	ErrReceiptLineUnknown = errors.New("procurement: the receipt names a line the order does not carry")
	// ErrReceiptLineDuplicate is returned when a receipt names the same line twice.
	ErrReceiptLineDuplicate = errors.New("procurement: a receipt names the same line twice")
	// ErrOverReceipt is returned when a receipt would receive more of a line than was ordered.
	ErrOverReceipt = errors.New("procurement: the receipt exceeds the ordered quantity")
	// ErrReceiptExists is returned when that delivery note is already recorded for the order.
	ErrReceiptExists = errors.New("procurement: a receipt with that reference is already recorded")
	// ErrReceiptNotFound is returned when a receipt does not exist.
	ErrReceiptNotFound = errors.New("procurement: goods receipt not found")
)

// ReceiptLine is one line of a delivery: how much of an order line arrived.
type ReceiptLine struct {
	LineNo        int
	QuantityMilli int64
}

// Receipt is one recorded goods receipt with its lines. ReceivedCents is the value the receipt
// converted, as it was converted — stored, not re-derived from today's prices.
type Receipt struct {
	ID            int64
	OrderID       int64
	Reference     string
	ReceivedOn    time.Time
	ReceivedBy    string
	Lines         []ReceiptLine
	ReceivedCents int64
}

// LineProgress is how far one order line has arrived: what was ordered, what has come in, and
// what is still open. It is derived from the receipts, never stored.
type LineProgress struct {
	LineNo        int
	Description   string
	OrderedMilli  int64
	ReceivedMilli int64
}

// OpenMilli is the quantity still to arrive.
func (p LineProgress) OpenMilli() int64 {
	if p.ReceivedMilli >= p.OrderedMilli {
		return 0
	}
	return p.OrderedMilli - p.ReceivedMilli
}

// Complete reports whether this line has arrived in full.
func (p LineProgress) Complete() bool { return p.OpenMilli() == 0 }

// ReceiptState is the goods-receipt state of an order: every line's progress, whether the order
// has arrived in full, and the receipts recorded so far.
type ReceiptState struct {
	OrderID  int64
	Progress []LineProgress
	Complete bool
	Receipts []Receipt
}

// ReceiptResult is what recording a receipt answers with: the receipt and the order's state as
// it now stands, so the screen neither re-derives the quantities nor asks again.
type ReceiptResult struct {
	Receipt Receipt
	State   ReceiptState
}

// lineByNo indexes an order's lines.
func lineByNo(order Order) map[int]Line {
	byNo := make(map[int]Line, len(order.Lines))
	for _, l := range order.Lines {
		byNo[l.LineNo] = l
	}
	return byNo
}

// NewReceipt validates a delivery against the order it belongs to, the quantities already
// received (`already` maps a line number to what has come in so far) and the value already
// converted (`convertedCents`, the sum of the order's earlier receipts). It returns the receipt
// to store, the value it converts, and whether the order is now complete.
//
// Everything is refused before anything is written: a blank reference or date, no lines, a
// non-positive quantity, a line the order does not carry, the same line twice, and a quantity
// that would receive more of a line than was ordered. An over-receipt is a refusal rather than a
// silent clamp, because a delivery of more than was ordered is a different transaction.
//
// The value it converts is the price of the quantities this receipt carries — except on the
// receipt that completes the order, which converts the remaining commitment instead, so the
// receipts add up to the order's gross exactly (see the note at the top of this file).
func NewReceipt(order Order, reference string, receivedOn time.Time, lines []ReceiptLine,
	already map[int]int64, convertedCents int64) (Receipt, int64, bool, error) {

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Receipt{}, 0, false, ErrReceiptReferenceRequired
	}
	if receivedOn.IsZero() {
		return Receipt{}, 0, false, ErrReceiptDateRequired
	}
	if len(lines) == 0 {
		return Receipt{}, 0, false, ErrNoReceiptLines
	}
	byNo := lineByNo(order)
	seen := make(map[int]bool, len(lines))
	received := make(map[int]int64, len(byNo))
	for no, qty := range already {
		received[no] = qty
	}
	var computed int64
	for _, l := range lines {
		if l.QuantityMilli <= 0 {
			return Receipt{}, 0, false, ErrReceiptQuantity
		}
		line, ok := byNo[l.LineNo]
		if !ok {
			return Receipt{}, 0, false, fmt.Errorf("%w: line %d", ErrReceiptLineUnknown, l.LineNo)
		}
		if seen[l.LineNo] {
			return Receipt{}, 0, false, fmt.Errorf("%w: line %d", ErrReceiptLineDuplicate, l.LineNo)
		}
		seen[l.LineNo] = true
		if received[l.LineNo]+l.QuantityMilli > line.QuantityMilli {
			return Receipt{}, 0, false, fmt.Errorf("%w: line %d ordered=%d received=%d now=%d",
				ErrOverReceipt, l.LineNo, line.QuantityMilli, received[l.LineNo], l.QuantityMilli)
		}
		received[l.LineNo] += l.QuantityMilli
		computed += ReceiptLineValue(line, l.QuantityMilli)
	}
	complete := IsFullyReceived(byNo, received)
	value := computed
	if remaining := remainingCommitment(order.GrossCents, convertedCents); complete || value > remaining {
		value = remaining
	}
	return Receipt{
		OrderID:    order.ID,
		Reference:  reference,
		ReceivedOn: receivedOn,
		Lines:      lines,
	}, value, complete, nil
}

// remainingCommitment is what of the order's gross has not been converted yet. It never goes
// negative: more converted than committed cannot happen through the domain, and clamping keeps a
// later receipt from converting a negative amount if it somehow did.
func remainingCommitment(grossCents, convertedCents int64) int64 {
	if remaining := grossCents - convertedCents; remaining > 0 {
		return remaining
	}
	return 0
}

// ReceiptLineValue is what a received quantity of one line is worth: its net at the line's unit
// price plus the line's own VAT.
func ReceiptLineValue(line Line, quantityMilli int64) int64 {
	net := LineNet(quantityMilli, line.UnitPriceCents)
	return net + roundHalfUp(net*line.VatRateBP, 10000)
}

// IsFullyReceived reports whether every order line has arrived in full.
func IsFullyReceived(lines map[int]Line, received map[int]int64) bool {
	if len(lines) == 0 {
		return false
	}
	for no, line := range lines {
		if received[no] < line.QuantityMilli {
			return false
		}
	}
	return true
}

// Progress returns every order line's state of arrival, in line order.
func Progress(order Order, received map[int]int64) []LineProgress {
	out := make([]LineProgress, 0, len(order.Lines))
	for _, l := range order.Lines {
		out = append(out, LineProgress{
			LineNo:        l.LineNo,
			Description:   l.Description,
			OrderedMilli:  l.QuantityMilli,
			ReceivedMilli: received[l.LineNo],
		})
	}
	return out
}
