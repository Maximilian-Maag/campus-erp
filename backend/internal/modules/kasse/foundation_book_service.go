package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// FoundationBookService implements the foundation-book slice (S6).
type FoundationBookService struct {
	repo  FoundationBookRepository
	audit AuditSink
}

// NewFoundationBookService wires the foundation-book service.
func NewFoundationBookService(repo FoundationBookRepository, sink AuditSink) *FoundationBookService {
	return &FoundationBookService{repo: repo, audit: sink}
}

// CreateBook validates and stores a foundation book.
func (s *FoundationBookService) CreateBook(ctx context.Context, foundationID int64, reference, currency, by string) (FoundationBook, error) {
	if err := requireActor(by); err != nil {
		return FoundationBook{}, err
	}
	book, err := NewFoundationBook(foundationID, reference, currency)
	if err != nil {
		return FoundationBook{}, err
	}
	book.CreatedBy = by
	saved, err := s.repo.CreateBook(ctx, book)
	if err != nil {
		return FoundationBook{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "foundation_book.create",
		Subject: bookSubject(saved.ID),
		After:   fmt.Sprintf("foundation=%d reference=%s", saved.FoundationID, saved.Reference),
	}); err != nil {
		return FoundationBook{}, err
	}
	return saved, nil
}

// Book appends a movement to a foundation book.
func (s *FoundationBookService) Book(ctx context.Context, bookID, amountCents int64, kind BookingKind, purpose, by string) (FoundationBooking, error) {
	if err := requireActor(by); err != nil {
		return FoundationBooking{}, err
	}
	booking, err := NewFoundationBooking(bookID, amountCents, kind, purpose)
	if err != nil {
		return FoundationBooking{}, err
	}
	saved, err := s.repo.InsertBooking(ctx, booking, by)
	if err != nil {
		return FoundationBooking{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "foundation_booking.create",
		Subject: bookSubject(bookID),
		After:   fmt.Sprintf("kind=%s amount_cents=%d", saved.Kind, saved.AmountCents),
	}); err != nil {
		return FoundationBooking{}, err
	}
	return saved, nil
}

// Bookings lists a book's movements.
func (s *FoundationBookService) Bookings(ctx context.Context, bookID int64) ([]FoundationBooking, error) {
	return s.repo.Bookings(ctx, bookID)
}

// Balance returns a book's derived balance.
func (s *FoundationBookService) Balance(ctx context.Context, bookID int64) (FoundationBalance, error) {
	return s.repo.Balance(ctx, bookID)
}

func bookSubject(id int64) string { return fmt.Sprintf("foundation_book/%d", id) }
