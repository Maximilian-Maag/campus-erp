package kasse

import (
	"errors"
	"strings"
	"time"
)

// Open receivable (offene Posten): a claim against a debtor under a mandate,
// due on a date, that a direct-debit collection run selects its items from
// (docs/22-collection-runs.md). The mandate supplies the debtor and the
// collection authorisation, so a receivable carries neither itself; its state
// is soft (open / collected / cancelled) because a collected item is what a run
// took, not something to delete (ADR-0006).

var (
	// ErrReceivableAmountNotPositive is returned for an amount at or below zero.
	ErrReceivableAmountNotPositive = errors.New("kasse: a receivable amount must be positive")
	// ErrReceivableCurrencyRequired is returned for a blank or malformed currency.
	ErrReceivableCurrencyRequired = errors.New("kasse: a receivable needs a three-letter currency")
	// ErrReceivableDueDateRequired is returned when the receivable has no due date.
	ErrReceivableDueDateRequired = errors.New("kasse: a receivable needs a due date")
	// ErrReceivablePurposeRequired is returned when the receivable has no purpose.
	ErrReceivablePurposeRequired = errors.New("kasse: a receivable needs a purpose")
	// ErrReceivablePurposeTooLong is returned for a purpose over 140 characters.
	ErrReceivablePurposeTooLong = errors.New("kasse: a receivable purpose must be at most 140 characters")
	// ErrReceivableNotFound is returned when a receivable does not exist.
	ErrReceivableNotFound = errors.New("kasse: receivable not found")
	// ErrReceivableNotOpen is returned when a receivable selected for collection
	// is no longer open.
	ErrReceivableNotOpen = errors.New("kasse: the receivable is not open")
	// ErrReceivableCurrencyMismatch is returned when a receivable's currency is
	// not the collecting account's.
	ErrReceivableCurrencyMismatch = errors.New("kasse: the receivable currency does not match the account")
	// ErrReceivableDueByRequired is returned when a run selects receivables with
	// no due-by date.
	ErrReceivableDueByRequired = errors.New("kasse: a due-by date is required to select receivables")
	// ErrReceivableDueByAfterCollection is returned when the due-by date is after
	// the collection date, which would collect an item not yet due.
	ErrReceivableDueByAfterCollection = errors.New("kasse: receivables are selected due on or before the collection date")
)

// maxPurposeLen matches the ISO 20022 Max140Text limit on the remittance
// information the collection carries.
const maxPurposeLen = 140

// ReceivableState is the soft state of an open item.
type ReceivableState string

const (
	// ReceivableOpen is an item still owed.
	ReceivableOpen ReceivableState = "open"
	// ReceivableCollected is an item a run has taken.
	ReceivableCollected ReceivableState = "collected"
	// ReceivableCancelled is an item written off without collection.
	ReceivableCancelled ReceivableState = "cancelled"
)

// Receivable is one open item (legacy: an offener Posten).
type Receivable struct {
	ID               int64
	MandateReference string
	AmountCents      int64
	Currency         string
	DueDate          time.Time
	Purpose          string
	State            ReceivableState
	CollectedRunID   *int64
	Source           string
	CreatedBy        string
}

// NewReceivable validates and builds an open receivable. The mandate reference
// is checked for shape here; whether the mandate exists is the repository's
// business (the foreign key enforces it).
func NewReceivable(mandateReference string, amountCents int64, currency string,
	dueDate time.Time, purpose, source string) (Receivable, error) {

	ref := strings.TrimSpace(mandateReference)
	if ref == "" {
		return Receivable{}, ErrMandateReferenceRequired
	}
	if len(ref) > maxMandateRefLen {
		return Receivable{}, ErrMandateReferenceTooLong
	}
	if amountCents <= 0 {
		return Receivable{}, ErrReceivableAmountNotPositive
	}
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if len(cur) != 3 {
		return Receivable{}, ErrReceivableCurrencyRequired
	}
	if dueDate.IsZero() {
		return Receivable{}, ErrReceivableDueDateRequired
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return Receivable{}, ErrReceivablePurposeRequired
	}
	if len([]rune(purpose)) > maxPurposeLen {
		return Receivable{}, ErrReceivablePurposeTooLong
	}
	return Receivable{
		MandateReference: ref,
		AmountCents:      amountCents,
		Currency:         cur,
		DueDate:          dueDate,
		Purpose:          purpose,
		State:            ReceivableOpen,
		Source:           strings.TrimSpace(source),
	}, nil
}
