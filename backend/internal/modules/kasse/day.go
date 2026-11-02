// Package kasse implements the Kassenbuchung (cash book) module — legacy `kbs`,
// the cash-book module. Slice S1 of the pilot (docs/plan/kbs-pilot.md).
package kasse

import (
	"errors"
	"time"
)

// DayStatus is the lifecycle state of a cash-book day.
type DayStatus string

const (
	// DayOpen accepts bookings.
	DayOpen DayStatus = "open"
	// DayClosed is finalised and rejects further bookings.
	DayClosed DayStatus = "closed"
)

var (
	// ErrNotFound is returned when a day does not exist.
	ErrNotFound = errors.New("kasse: not found")
	// ErrDayExists is returned when a day for that business date already exists.
	ErrDayExists = errors.New("kasse: a day for that business date already exists")
	// ErrDayClosed is returned when a booking targets a closed day.
	ErrDayClosed = errors.New("kasse: day is already closed")
	// ErrAmountNotPositive is returned for a zero or negative amount.
	ErrAmountNotPositive = errors.New("kasse: amount must be greater than zero")
	// ErrPurposeRequired is returned when the purpose is blank.
	ErrPurposeRequired = errors.New("kasse: purpose is required")
	// ErrUnknownKind is returned for a booking kind other than income/expense.
	ErrUnknownKind = errors.New("kasse: unknown booking kind")
	// ErrLegacyRefRequired is returned by an import when a booking has no legacy
	// reference to key its idempotent upsert on.
	ErrLegacyRefRequired = errors.New("kasse: a legacy reference is required")
)

// Day is one cash-book business day.
type Day struct {
	ID           int64
	BusinessDate time.Time
	Status       DayStatus
	OpeningCents int64
	ClosingCents *int64
}

// CanBook reports whether the day accepts bookings.
func (d Day) CanBook() error {
	if d.Status == DayClosed {
		return ErrDayClosed
	}
	return nil
}

// IsClosed reports whether the day is finalised.
func (d Day) IsClosed() bool { return d.Status == DayClosed }
