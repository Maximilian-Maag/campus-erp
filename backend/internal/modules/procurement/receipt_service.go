package procurement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// Recording a goods receipt (docs/12). The order must be approved: a draft committed nothing, a
// received order has arrived in full, and a cancelled one is no longer a liability. What the
// receipt converts is the price of the quantities it carries, except on the receipt that
// completes the order, which converts the remaining commitment — so an order's receipts add up to
// its gross exactly (see the note at the top of receipt.go).
//
// The value is converted **before** the receipt is stored, in the same order the invoice-driven
// match uses: a refused conversion must leave the order exactly as it was, with no receipt to
// explain. A delivery note the order already carries is refused up front, so the common duplicate
// is caught before the budget moves; the unique key remains the guarantee under a race.

// RecordReceipt records a delivery against an approved order, converting what arrived from a
// reservation into a consumption. It answers with the receipt and the order's state afterwards.
func (s *OrderService) RecordReceipt(ctx context.Context, orderID int64, reference string,
	receivedOn time.Time, lines []ReceiptLine, by string) (ReceiptResult, error) {

	if strings.TrimSpace(by) == "" {
		return ReceiptResult{}, ErrActorRequired
	}
	order, err := s.repo.Get(ctx, orderID)
	if err != nil {
		return ReceiptResult{}, err
	}
	if order.Status != StatusApproved {
		return ReceiptResult{}, fmt.Errorf("%w: order %s is %s", ErrBadTransition, order.Number, order.Status)
	}
	receipts, err := s.repo.Receipts(ctx, orderID)
	if err != nil {
		return ReceiptResult{}, err
	}
	wanted := strings.TrimSpace(reference)
	for _, existing := range receipts {
		if existing.Reference == wanted {
			return ReceiptResult{}, fmt.Errorf("%w: %s", ErrReceiptExists, wanted)
		}
	}
	received, converted, err := s.repo.GoodsReceived(ctx, orderID)
	if err != nil {
		return ReceiptResult{}, err
	}
	// Invoices may have converted part of the commitment already (docs/12): a delivery converts what
	// it is worth out of what is left, so the gross is never consumed twice.
	receipt, value, complete, err := NewReceipt(order, reference, receivedOn, lines, received, converted+order.InvoicedCents)
	if err != nil {
		return ReceiptResult{}, err
	}
	if value > 0 {
		// Release the reserved part and consume it, in one step, so the position's available
		// Mittel are unchanged by the conversion rather than counted twice. A delivery of goods
		// that cost nothing (a unit price of zero) is recorded without a budget movement.
		if _, err := s.budget.Post(ctx, order.BudgetPositionID, budget.Release, value, wanted+":release", by); err != nil {
			return ReceiptResult{}, err
		}
		if _, err := s.budget.Post(ctx, order.BudgetPositionID, budget.Actual, value, wanted, by); err != nil {
			return ReceiptResult{}, err
		}
	}
	receipt.ReceivedCents = value
	receipt.ReceivedBy = by
	saved, err := s.repo.InsertReceipt(ctx, receipt)
	if err != nil {
		return ReceiptResult{}, err
	}
	// The order has arrived in full: it becomes received, so the state says what happened
	// rather than leaving a fully delivered order looking open.
	if complete {
		if _, err := s.repo.SetStatus(ctx, orderID, StatusReceived, by); err != nil {
			return ReceiptResult{}, err
		}
		order.Status = StatusReceived
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "purchase_order.receipt",
		Subject: orderSubject(orderID),
		After: fmt.Sprintf("number=%s reference=%s value_cents=%d lines=%d complete=%t status=%s",
			order.Number, saved.Reference, saved.ReceivedCents, len(saved.Lines), complete, order.Status),
	}); err != nil {
		return ReceiptResult{}, err
	}
	state, err := s.receiptState(ctx, order)
	if err != nil {
		return ReceiptResult{}, err
	}
	return ReceiptResult{Receipt: saved, State: state}, nil
}

// ReceiptState is the goods-receipt state of an order — the throttled read the screen shows
// before it records a delivery. An order with no receipt is a state with no receipts and the
// whole of every line open, not a not-found.
func (s *OrderService) ReceiptState(ctx context.Context, orderID int64) (ReceiptState, error) {
	order, err := s.repo.Get(ctx, orderID)
	if err != nil {
		return ReceiptState{}, err
	}
	return s.receiptState(ctx, order)
}

// receiptState derives the state from the stored receipts. It reads the progress and the
// receipts in two queries rather than one join, because the two are independent lists and a
// join would multiply the receipts by their lines.
func (s *OrderService) receiptState(ctx context.Context, order Order) (ReceiptState, error) {
	received, _, err := s.repo.GoodsReceived(ctx, order.ID)
	if err != nil {
		return ReceiptState{}, err
	}
	receipts, err := s.repo.Receipts(ctx, order.ID)
	if err != nil {
		return ReceiptState{}, err
	}
	return ReceiptState{
		OrderID:  order.ID,
		Progress: Progress(order, received),
		Complete: IsFullyReceived(lineByNo(order), received),
		Receipts: receipts,
	}, nil
}
