package kasse

import "strings"

// BookingKind distinguishes money in from money out.
type BookingKind string

const (
	// Income increases the day balance.
	Income BookingKind = "income"
	// Expense decreases the day balance.
	Expense BookingKind = "expense"
)

// Booking is a single cash movement on a day.
type Booking struct {
	ID          int64
	DayID       int64
	Kind        BookingKind
	AmountCents int64
	Purpose     string
}

// NewBooking validates and constructs a booking for the given day.
func NewBooking(day Day, kind BookingKind, amountCents int64, purpose string) (Booking, error) {
	if err := day.CanBook(); err != nil {
		return Booking{}, err
	}
	if kind != Income && kind != Expense {
		return Booking{}, ErrUnknownKind
	}
	if amountCents <= 0 {
		return Booking{}, ErrAmountNotPositive
	}
	trimmed := strings.TrimSpace(purpose)
	if trimmed == "" {
		return Booking{}, ErrPurposeRequired
	}
	return Booking{DayID: day.ID, Kind: kind, AmountCents: amountCents, Purpose: trimmed}, nil
}

// Signed returns the booking's effect on the day balance.
func (b Booking) Signed() int64 {
	if b.Kind == Expense {
		return -b.AmountCents
	}
	return b.AmountCents
}

// Balance computes a day's closing balance from its opening balance and bookings.
func Balance(openingCents int64, bookings []Booking) int64 {
	total := openingCents
	for _, b := range bookings {
		total += b.Signed()
	}
	return total
}
