package kasse

import (
	"context"
	"fmt"
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

// Day returns a day by id.
func (s *Service) Day(ctx context.Context, id int64) (Day, error) { return s.repo.GetDay(ctx, id) }
