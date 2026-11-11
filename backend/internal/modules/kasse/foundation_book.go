package kasse

import (
	"errors"
	"strings"
)

// Slice S6: a separate book per foundation, so one foundation's movements can
// never be mixed with another's.

var (
	// ErrFoundationBookReferenceRequired is returned when a book has no reference.
	ErrFoundationBookReferenceRequired = errors.New("kasse: a book reference is required")
	// ErrFoundationBookExists is returned when the (foundation, reference) pair is taken.
	ErrFoundationBookExists = errors.New("kasse: a book with that reference already exists for the foundation")
	// ErrFoundationBookNotFound is returned when a book does not exist.
	ErrFoundationBookNotFound = errors.New("kasse: foundation book not found")
	// ErrFoundationRequired is returned when the foundation is missing.
	ErrFoundationRequired = errors.New("kasse: a foundation is required")
	// ErrCarryOverNotAllowed is returned when the foundation's record does not allow its money to be
	// carried into the next year.
	ErrCarryOverNotAllowed = errors.New("kasse: the foundation does not carry its money over")
	// ErrCarryOverNothing is returned when a book's balance is zero: there is nothing to carry.
	ErrCarryOverNothing = errors.New("kasse: the book has no balance to carry over")
	// ErrCarryOverTargetMissing is returned when the next year's foundation record does not exist.
	// Master data is not a side effect of a posting, so the operator opens the year first.
	ErrCarryOverTargetMissing = errors.New("kasse: the next year's foundation does not exist")
	// ErrCarryOverTargetNotEmpty is returned when the next year's book already carries movements, so
	// posting an opening balance into it would count the money twice.
	ErrCarryOverTargetNotEmpty = errors.New("kasse: the next year's book already carries movements")
)

// CarryOver is the result of closing one year's book into the next: the book that was closed, the
// book that was opened, the opening movement, and the net that moved.
type CarryOver struct {
	Source  FoundationBook
	Target  FoundationBook
	Booking FoundationBooking
	// Year is the year that was closed; the target book belongs to Year+1.
	Year  int
	Cents int64
}

// FoundationBook is one foundation's own book.
type FoundationBook struct {
	ID           int64
	FoundationID int64
	Reference    string
	Currency     string
	CreatedBy    string
}

// FoundationBooking is one append-only movement in a foundation book.
type FoundationBooking struct {
	ID          int64
	BookID      int64
	Kind        BookingKind
	AmountCents int64
	Purpose     string
	BookedBy    string
}

// FoundationBalance is the derived balance of a book.
type FoundationBalance struct {
	BookID       int64
	IncomeCents  int64
	ExpenseCents int64
}

// Net returns income minus expense.
func (b FoundationBalance) Net() int64 { return b.IncomeCents - b.ExpenseCents }

// NewFoundationBook validates and builds a book.
func NewFoundationBook(foundationID int64, reference, currency string) (FoundationBook, error) {
	if foundationID <= 0 {
		return FoundationBook{}, ErrFoundationRequired
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return FoundationBook{}, ErrFoundationBookReferenceRequired
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = "EUR"
	}
	if len(currency) != 3 {
		return FoundationBook{}, ErrBadCurrency
	}
	return FoundationBook{FoundationID: foundationID, Reference: reference, Currency: currency}, nil
}

// NewFoundationBooking validates and builds a movement.
func NewFoundationBooking(bookID, amountCents int64, kind BookingKind, purpose string) (FoundationBooking, error) {
	if bookID <= 0 {
		return FoundationBooking{}, ErrFoundationBookNotFound
	}
	if kind != Income && kind != Expense {
		return FoundationBooking{}, ErrUnknownKind
	}
	if amountCents <= 0 {
		return FoundationBooking{}, ErrAmountNotPositive
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return FoundationBooking{}, ErrPurposeRequired
	}
	return FoundationBooking{BookID: bookID, Kind: kind, AmountCents: amountCents, Purpose: purpose}, nil
}
