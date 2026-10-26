// Package budget implements the budget (Mittelbewirtschaftung) slice: budget
// positions with append-only postings and Mittelüberwachung (a posting may not
// drive the available amount below zero).
package budget

import (
	"errors"
	"strings"
)

// Validation errors.
var (
	ErrYearRequired      = errors.New("budget: a year is required")
	ErrChapterRequired   = errors.New("budget: a chapter is required")
	ErrTitleRequired     = errors.New("budget: a title is required")
	ErrAllocatedNegative = errors.New("budget: the allocated amount must not be negative")
	ErrAmountNotPositive = errors.New("budget: the amount must be positive")
	ErrUnknownKind       = errors.New("budget: unknown posting kind")
	ErrActorRequired     = errors.New("budget: an actor is required")
	ErrPositionExists    = errors.New("budget: a position with that year, chapter and title already exists")
	ErrPositionNotFound  = errors.New("budget: budget position not found")
	ErrBudgetExceeded    = errors.New("budget: the posting exceeds the available budget")
)

// PostingKind distinguishes a reservation from a consumption.
type PostingKind string

const (
	// Commitment reserves budget (Verpflichtung).
	Commitment PostingKind = "commitment"
	// Release frees a reservation (Aufhebung).
	Release PostingKind = "release"
	// Actual consumes budget (Ist-Buchung).
	Actual PostingKind = "actual"
)

// Position is a budget title (Haushaltstitel).
type Position struct {
	ID             int64
	Year           int
	Chapter        string
	Title          string
	Description    string
	AllocatedCents int64
	CreatedBy      string
}

// Posting is one append-only movement on a position.
type Posting struct {
	ID          int64
	PositionID  int64
	Kind        PostingKind
	AmountCents int64
	Reference   string
	BookedBy    string
}

// Balance is the derived state of a position.
type Balance struct {
	PositionID     int64
	AllocatedCents int64
	CommittedCents int64
	ActualCents    int64
}

// AvailableCents is the amount still free: allocated minus commitments minus actuals.
func (b Balance) AvailableCents() int64 {
	return b.AllocatedCents - b.CommittedCents - b.ActualCents
}

// NewPosition validates and builds a position.
func NewPosition(year int, chapter, title, description string, allocatedCents int64) (Position, error) {
	if year < 1900 || year > 2999 {
		return Position{}, ErrYearRequired
	}
	chapter = strings.TrimSpace(chapter)
	if chapter == "" {
		return Position{}, ErrChapterRequired
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Position{}, ErrTitleRequired
	}
	if allocatedCents < 0 {
		return Position{}, ErrAllocatedNegative
	}
	return Position{
		Year: year, Chapter: chapter, Title: title,
		Description: strings.TrimSpace(description), AllocatedCents: allocatedCents,
	}, nil
}

// NewPosting validates and builds a posting.
func NewPosting(positionID int64, kind PostingKind, amountCents int64, reference string) (Posting, error) {
	if positionID <= 0 {
		return Posting{}, ErrPositionNotFound
	}
	if kind != Commitment && kind != Release && kind != Actual {
		return Posting{}, ErrUnknownKind
	}
	if amountCents <= 0 {
		return Posting{}, ErrAmountNotPositive
	}
	return Posting{
		PositionID: positionID, Kind: kind, AmountCents: amountCents,
		Reference: strings.TrimSpace(reference),
	}, nil
}

// CheckAvailable reports ErrBudgetExceeded when a posting of amountCents of the
// given kind would drive the available amount below zero. A release never can.
func CheckAvailable(b Balance, kind PostingKind, amountCents int64) error {
	if kind == Release {
		return nil
	}
	if b.AvailableCents()-amountCents < 0 {
		return ErrBudgetExceeded
	}
	return nil
}
