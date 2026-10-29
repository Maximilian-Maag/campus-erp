package kasse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// Repository is the persistence port for the cash book.
type Repository interface {
	OpenDay(ctx context.Context, date time.Time, openingCents int64, by string) (Day, error)
	GetDay(ctx context.Context, id int64) (Day, error)
	Bookings(ctx context.Context, dayID int64) ([]Booking, error)
	InsertBooking(ctx context.Context, b Booking, by string) (Booking, error)
	CloseDay(ctx context.Context, id, closingCents int64, by string) (Day, error)

	// Import path (counter-book ETL): idempotent upserts keyed by date and the
	// booking's legacy reference (docs/legacy/kbs-cashbook.md).
	UpsertImportedDay(ctx context.Context, date time.Time, by string) (Day, error)
	UpsertImportedBooking(ctx context.Context, b Booking, by, legacyRef string) (Booking, error)
	SetDayClosing(ctx context.Context, id, closingCents int64, by string) (Day, error)
}

// AuditSink records every state change (ADR-0006).
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// Service implements the cash-book use cases. Every mutation emits an audit event.
type Service struct {
	repo  Repository
	audit AuditSink
}

// NewService wires a cash-book service.
func NewService(repo Repository, sink AuditSink) *Service {
	return &Service{repo: repo, audit: sink}
}

// OpenDay opens a cash-book day.
func (s *Service) OpenDay(ctx context.Context, date time.Time, openingCents int64, by string) (Day, error) {
	d, err := s.repo.OpenDay(ctx, date, openingCents, by)
	if err != nil {
		return Day{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cashbook.open",
		Subject: fmt.Sprintf("day/%d", d.ID),
		After:   fmt.Sprintf("opening_cents=%d", openingCents),
	}); err != nil {
		return Day{}, err
	}
	return d, nil
}

// Book records a booking on an open day.
func (s *Service) Book(ctx context.Context, dayID int64, kind BookingKind, amountCents int64, purpose, by string) (Booking, error) {
	d, err := s.repo.GetDay(ctx, dayID)
	if err != nil {
		return Booking{}, err
	}
	b, err := NewBooking(d, kind, amountCents, purpose)
	if err != nil {
		return Booking{}, err
	}
	saved, err := s.repo.InsertBooking(ctx, b, by)
	if err != nil {
		return Booking{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cashbook.book",
		Subject: fmt.Sprintf("day/%d", dayID),
		After:   fmt.Sprintf("kind=%s amount_cents=%d", kind, amountCents),
	}); err != nil {
		return Booking{}, err
	}
	return saved, nil
}

// CloseDay finalises a day, storing its computed closing balance.
func (s *Service) CloseDay(ctx context.Context, dayID int64, by string) (Day, error) {
	d, err := s.repo.GetDay(ctx, dayID)
	if err != nil {
		return Day{}, err
	}
	if d.IsClosed() {
		return Day{}, ErrDayClosed
	}
	bookings, err := s.repo.Bookings(ctx, dayID)
	if err != nil {
		return Day{}, err
	}
	closing := Balance(d.OpeningCents, bookings)
	closed, err := s.repo.CloseDay(ctx, dayID, closing, by)
	if err != nil {
		return Day{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cashbook.close",
		Subject: fmt.Sprintf("day/%d", dayID),
		After:   fmt.Sprintf("closing_cents=%d", closing),
	}); err != nil {
		return Day{}, err
	}
	return closed, nil
}

// CounterBookEntry is one legacy counter-book booking to import.
type CounterBookEntry struct {
	LegacyRef   string
	Kind        BookingKind
	AmountCents int64
	Purpose     string
}

// ImportCounterBook imports one day's counter-book bookings (legacy
// kbs.schalterbuch). The whole batch is validated before any write; the day is
// created closed with a computed closing balance; bookings upsert by legacy
// reference, so a re-run converges. One audit event per day.
func (s *Service) ImportCounterBook(ctx context.Context, date time.Time, entries []CounterBookEntry, by string) (int, error) {
	if err := requireActor(by); err != nil {
		return 0, err
	}
	for _, e := range entries {
		if strings.TrimSpace(e.LegacyRef) == "" {
			return 0, fmt.Errorf("kasse: counter-book entry: %w", ErrLegacyRefRequired)
		}
		if e.Kind != Income && e.Kind != Expense {
			return 0, ErrUnknownKind
		}
		if e.AmountCents <= 0 {
			return 0, ErrAmountNotPositive
		}
		if strings.TrimSpace(e.Purpose) == "" {
			return 0, ErrPurposeRequired
		}
	}
	d, err := s.repo.UpsertImportedDay(ctx, date, by)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		b := Booking{DayID: d.ID, Kind: e.Kind, AmountCents: e.AmountCents, Purpose: strings.TrimSpace(e.Purpose)}
		if _, err := s.repo.UpsertImportedBooking(ctx, b, by, e.LegacyRef); err != nil {
			return 0, err
		}
	}
	bookings, err := s.repo.Bookings(ctx, d.ID)
	if err != nil {
		return 0, err
	}
	closing := Balance(d.OpeningCents, bookings)
	if _, err := s.repo.SetDayClosing(ctx, d.ID, closing, by); err != nil {
		return 0, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cashbook.import",
		Subject: fmt.Sprintf("day/%d", d.ID),
		After:   fmt.Sprintf("date=%s bookings=%d closing_cents=%d", date.Format(time.DateOnly), len(entries), closing),
	}); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Day returns a day by id.
func (s *Service) Day(ctx context.Context, id int64) (Day, error) { return s.repo.GetDay(ctx, id) }
