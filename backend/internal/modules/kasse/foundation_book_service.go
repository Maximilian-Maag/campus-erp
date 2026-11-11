package kasse

import (
	"context"
	"fmt"
	"strings"

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

// Books lists a foundation's books. The foundation is the scope, so a caller only ever sees
// the books of the foundation it named.
func (s *FoundationBookService) Books(ctx context.Context, foundationID int64) ([]FoundationBook, error) {
	return s.repo.Books(ctx, foundationID)
}

// CarryOver closes one year's book into the next: it posts the book's net as the opening movement of
// the same foundation's book in the following year.
//
// The rules, in the order they refuse:
//
//   - the foundation's record must allow a carry-over at all (`carry_over`) — a foundation whose
//     money is spent in its year has nothing to open the next with;
//   - the book must have a balance: a net of zero carries nothing;
//   - the next year's *foundation record* must exist. Master data is not a side effect of a posting,
//     so the operator opens the year first rather than having one invented here;
//   - the next year's book must not already carry movements, which is what makes a second carry-over
//     of the same year a refusal rather than a doubled opening balance.
//
// The source book is not touched: the years' books are append-only records of what happened, and
// closing a year means opening the next with the net, not editing the one that ended.
func (s *FoundationBookService) CarryOver(ctx context.Context, bookID int64, reference, by string) (CarryOver, error) {
	if err := requireActor(by); err != nil {
		return CarryOver{}, err
	}
	source, err := s.repo.GetBook(ctx, bookID)
	if err != nil {
		return CarryOver{}, err
	}
	foundation, err := s.repo.FoundationByID(ctx, source.FoundationID)
	if err != nil {
		return CarryOver{}, err
	}
	if !foundation.CarryOver {
		return CarryOver{}, fmt.Errorf("%w: foundation %d/%s", ErrCarryOverNotAllowed, foundation.Year, foundation.Number)
	}
	bal, err := s.repo.Balance(ctx, bookID)
	if err != nil {
		return CarryOver{}, err
	}
	net := bal.Net()
	if net == 0 {
		return CarryOver{}, fmt.Errorf("%w: book %s", ErrCarryOverNothing, source.Reference)
	}
	next, ok, err := s.repo.FoundationByYearNumber(ctx, foundation.Year+1, foundation.Number)
	if err != nil {
		return CarryOver{}, err
	}
	if !ok {
		return CarryOver{}, fmt.Errorf("%w: %d/%s", ErrCarryOverTargetMissing, foundation.Year+1, foundation.Number)
	}
	ref := strings.TrimSpace(reference)
	if ref == "" {
		ref = source.Reference
	}
	target, found, err := s.repo.BookByReference(ctx, next.ID, ref)
	if err != nil {
		return CarryOver{}, err
	}
	if found {
		// An existing book is reused only when it is still empty: a book that already carries
		// movements has been opened, and posting into it again would count the money twice.
		existing, err := s.repo.Bookings(ctx, target.ID)
		if err != nil {
			return CarryOver{}, err
		}
		if len(existing) > 0 {
			return CarryOver{}, fmt.Errorf("%w: %s", ErrCarryOverTargetNotEmpty, target.Reference)
		}
	} else {
		target, err = s.repo.CreateBook(ctx, FoundationBook{
			FoundationID: next.ID,
			Reference:    ref,
			Currency:     source.Currency,
			CreatedBy:    by,
		})
		if err != nil {
			return CarryOver{}, err
		}
	}
	kind, amount := Income, net
	if net < 0 {
		kind, amount = Expense, -net
	}
	booking, err := NewFoundationBooking(target.ID, amount, kind,
		fmt.Sprintf("Übertrag aus %d (%s)", foundation.Year, source.Reference))
	if err != nil {
		return CarryOver{}, err
	}
	saved, err := s.repo.InsertBooking(ctx, booking, by)
	if err != nil {
		return CarryOver{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "foundation.carry_over",
		Subject: bookSubject(source.ID),
		After: fmt.Sprintf("from=%d/%s to=%d/%s cents=%d target_book=%d",
			foundation.Year, source.Reference, next.Year, target.Reference, net, target.ID),
	}); err != nil {
		return CarryOver{}, err
	}
	return CarryOver{Source: source, Target: target, Booking: saved, Year: foundation.Year, Cents: net}, nil
}

func bookSubject(id int64) string { return fmt.Sprintf("foundation_book/%d", id) }
