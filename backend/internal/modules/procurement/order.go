// Package procurement implements purchase orders (Bestellungen). Approving an
// order commits budget through the budget module; cancelling releases it.
package procurement

import (
	"errors"
	"fmt"
	"strings"
)

// Validation errors.
var (
	ErrNumberRequired         = errors.New("procurement: an order number is required")
	ErrSupplierRequired       = errors.New("procurement: a supplier is required")
	ErrBudgetPositionRequired = errors.New("procurement: a budget position is required")
	ErrCurrency               = errors.New("procurement: the currency must be a 3-letter ISO code")
	ErrNoLines                = errors.New("procurement: at least one line is required")
	ErrLineDescription        = errors.New("procurement: a line description is required")
	ErrLineQuantity           = errors.New("procurement: a line quantity must be positive")
	ErrLineAmount             = errors.New("procurement: a line unit price must not be negative")
	ErrVATCategoryRequired    = errors.New("procurement: a line VAT category is required")
	ErrLineNetMismatch        = errors.New("procurement: a line net amount does not match quantity x unit price")
	ErrActorRequired          = errors.New("procurement: an actor is required")
	ErrOrderExists            = errors.New("procurement: an order with that number already exists")
	ErrOrderNotFound          = errors.New("procurement: purchase order not found")
	ErrBadTransition          = errors.New("procurement: the order is not in a state that allows this step")
	// ErrMatchAmountMismatch is returned when an inbound invoice's gross differs from its
	// order's: the two-way match the receipt is built on.
	ErrMatchAmountMismatch = errors.New("procurement: the invoice does not match the order's amount")
	// ErrUnknownStatus is returned for a status that is not part of the lifecycle.
	ErrUnknownStatus = errors.New("procurement: unknown order status")
)

// Status is the order's lifecycle state.
type Status string

const (
	// StatusDraft is a new, unapproved order.
	StatusDraft Status = "draft"
	// StatusApproved has committed budget.
	StatusApproved Status = "approved"
	// StatusReceived has had its goods received.
	StatusReceived Status = "received"
	// StatusCancelled is abandoned; its commitment is released.
	StatusCancelled Status = "cancelled"
)

// Statuses returns the lifecycle states, in lifecycle order.
func Statuses() []Status {
	return []Status{StatusDraft, StatusApproved, StatusReceived, StatusCancelled}
}

// ParseStatus reads a status from an argument, rejecting anything outside the lifecycle rather
// than silently listing everything.
func ParseStatus(value string) (Status, error) {
	s := Status(strings.TrimSpace(value))
	for _, known := range Statuses() {
		if s == known {
			return s, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownStatus, value)
}

// OrderSummary is a purchase order as a list shows it: the header, without the lines. That is
// what an overview of open orders needs, and it avoids loading lines the caller will not read.
type OrderSummary struct {
	ID               int64
	Number           string
	Supplier         string
	BudgetPositionID int64
	Currency         string
	NetCents         int64
	VatCents         int64
	GrossCents       int64
	Status           Status
}

// Line is one order line.
type Line struct {
	LineNo         int
	Description    string
	QuantityMilli  int64
	UnitPriceCents int64
	NetCents       int64
	VatCategory    string
	VatRateBP      int64
}

// Order is a purchase order with its lines.
type Order struct {
	ID               int64
	Number           string
	Supplier         string
	BudgetPositionID int64
	Currency         string
	Lines            []Line
	NetCents         int64
	VatCents         int64
	GrossCents       int64
	Status           Status
	CreatedBy        string
}

func roundHalfUp(numerator, denominator int64) int64 {
	if numerator >= 0 {
		return (numerator + denominator/2) / denominator
	}
	return -((-numerator + denominator/2) / denominator)
}

// LineNet returns the line's net amount from quantity and unit price.
func LineNet(quantityMilli, unitPriceCents int64) int64 {
	return roundHalfUp(quantityMilli*unitPriceCents, 1000)
}

// ComputeTotals returns the net, VAT and gross totals for the lines.
func ComputeTotals(lines []Line) (net, vat, gross int64) {
	type key struct {
		cat  string
		rate int64
	}
	taxable := map[key]int64{}
	var order []key
	for _, l := range lines {
		net += l.NetCents
		k := key{l.VatCategory, l.VatRateBP}
		if _, ok := taxable[k]; !ok {
			order = append(order, k)
		}
		taxable[k] += l.NetCents
	}
	for _, k := range order {
		vat += roundHalfUp(taxable[k]*k.rate, 10000)
	}
	return net, vat, net + vat
}

// NewOrder validates and builds a draft order.
func NewOrder(number, supplier string, budgetPositionID int64, currency string, lines []Line) (Order, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return Order{}, ErrNumberRequired
	}
	supplier = strings.TrimSpace(supplier)
	if supplier == "" {
		return Order{}, ErrSupplierRequired
	}
	if budgetPositionID <= 0 {
		return Order{}, ErrBudgetPositionRequired
	}
	if len(currency) != 3 {
		return Order{}, ErrCurrency
	}
	if len(lines) == 0 {
		return Order{}, ErrNoLines
	}
	for _, l := range lines {
		if strings.TrimSpace(l.Description) == "" {
			return Order{}, ErrLineDescription
		}
		if l.QuantityMilli <= 0 {
			return Order{}, ErrLineQuantity
		}
		if l.UnitPriceCents < 0 {
			return Order{}, ErrLineAmount
		}
		if strings.TrimSpace(l.VatCategory) == "" {
			return Order{}, ErrVATCategoryRequired
		}
		if l.NetCents != LineNet(l.QuantityMilli, l.UnitPriceCents) {
			return Order{}, ErrLineNetMismatch
		}
	}
	net, vat, gross := ComputeTotals(lines)
	return Order{
		Number: number, Supplier: supplier, BudgetPositionID: budgetPositionID,
		Currency: currency, Lines: lines, NetCents: net, VatCents: vat, GrossCents: gross,
		Status: StatusDraft,
	}, nil
}
