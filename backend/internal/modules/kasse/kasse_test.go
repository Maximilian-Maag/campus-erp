package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

func openDay() Day {
	return Day{ID: 7, BusinessDate: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), Status: DayOpen, OpeningCents: 10000}
}

func TestNewBookingRejectsClosedDay(t *testing.T) {
	d := openDay()
	d.Status = DayClosed
	if _, err := NewBooking(d, Income, 500, "fee"); !errors.Is(err, ErrDayClosed) {
		t.Fatalf("want ErrDayClosed, got %v", err)
	}
}

func TestNewBookingValidation(t *testing.T) {
	cases := []struct {
		name    string
		kind    BookingKind
		amount  int64
		purpose string
		want    error
	}{
		{"unknown kind", "transfer", 100, "x", ErrUnknownKind},
		{"zero amount", Income, 0, "x", ErrAmountNotPositive},
		{"negative amount", Expense, -5, "x", ErrAmountNotPositive},
		{"blank purpose", Income, 100, "   ", ErrPurposeRequired},
		{"empty purpose", Income, 100, "", ErrPurposeRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewBooking(openDay(), c.kind, c.amount, c.purpose); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestNewBookingTrimsPurpose(t *testing.T) {
	b, err := NewBooking(openDay(), Income, 250, "  Gebühr  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Purpose != "Gebühr" {
		t.Errorf("purpose not trimmed: %q", b.Purpose)
	}
	if b.DayID != 7 {
		t.Errorf("day id: got %d want 7", b.DayID)
	}
}

func TestSigned(t *testing.T) {
	if got := (Booking{Kind: Income, AmountCents: 500}).Signed(); got != 500 {
		t.Errorf("income signed = %d, want 500", got)
	}
	if got := (Booking{Kind: Expense, AmountCents: 500}).Signed(); got != -500 {
		t.Errorf("expense signed = %d, want -500", got)
	}
}

func TestBalance(t *testing.T) {
	bookings := []Booking{
		{Kind: Income, AmountCents: 500},
		{Kind: Expense, AmountCents: 200},
		{Kind: Income, AmountCents: 50},
	}
	if got := Balance(1000, bookings); got != 1350 {
		t.Errorf("Balance = %d, want 1350", got)
	}
	if got := Balance(1000, nil); got != 1000 {
		t.Errorf("empty Balance = %d, want 1000", got)
	}
}

// ── service, with fakes ─────────────────────────────────────────────────────

type fakeRepo struct {
	day      Day
	bookings []Booking
	inserted []Booking
	closed   bool
	err      error
}

func (f *fakeRepo) OpenDay(_ context.Context, date time.Time, opening int64, _ string) (Day, error) {
	if f.err != nil {
		return Day{}, f.err
	}
	f.day = Day{ID: 1, BusinessDate: date, Status: DayOpen, OpeningCents: opening}
	return f.day, nil
}

func (f *fakeRepo) GetDay(_ context.Context, _ int64) (Day, error) {
	if f.err != nil {
		return Day{}, f.err
	}
	return f.day, nil
}

func (f *fakeRepo) Bookings(_ context.Context, _ int64) ([]Booking, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bookings, nil
}

func (f *fakeRepo) InsertBooking(_ context.Context, b Booking, _ string) (Booking, error) {
	if f.err != nil {
		return Booking{}, f.err
	}
	b.ID = int64(len(f.inserted) + 1)
	f.inserted = append(f.inserted, b)
	f.bookings = append(f.bookings, b)
	return b, nil
}

func (f *fakeRepo) CloseDay(_ context.Context, _ int64, closing int64, _ string) (Day, error) {
	if f.err != nil {
		return Day{}, f.err
	}
	f.closed = true
	f.day.Status = DayClosed
	f.day.ClosingCents = &closing
	return f.day, nil
}

type failingAudit struct{ err error }

func (f failingAudit) Append(context.Context, audit.Event) (audit.Event, error) {
	return audit.Event{}, f.err
}

type fakeAudit struct{ events []audit.Event }

func (f *fakeAudit) Append(_ context.Context, e audit.Event) (audit.Event, error) {
	f.events = append(f.events, e)
	return e, nil
}

func TestServiceCloseDayComputesBalanceAndAudits(t *testing.T) {
	repo := &fakeRepo{day: Day{ID: 1, Status: DayOpen, OpeningCents: 1000}}
	sink := &fakeAudit{}
	svc := NewService(repo, sink)

	if _, err := svc.Book(context.Background(), 1, Income, 500, "fee", "alice"); err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := svc.Book(context.Background(), 1, Expense, 200, "payout", "alice"); err != nil {
		t.Fatalf("book: %v", err)
	}
	closed, err := svc.CloseDay(context.Background(), 1, "bob")
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.ClosingCents == nil || *closed.ClosingCents != 1300 {
		t.Fatalf("closing balance wrong: %+v", closed.ClosingCents)
	}
	if len(sink.events) != 3 {
		t.Fatalf("want 3 audit events, got %d", len(sink.events))
	}
	if sink.events[2].Action != "cashbook.close" {
		t.Errorf("last audit action = %q", sink.events[2].Action)
	}
	if !strings.HasPrefix(sink.events[0].Subject, "day/") {
		t.Errorf("audit subject = %q", sink.events[0].Subject)
	}
}

func TestServiceBookRejectsClosedDay(t *testing.T) {
	repo := &fakeRepo{day: Day{ID: 1, Status: DayClosed, OpeningCents: 0}}
	svc := NewService(repo, &fakeAudit{})
	if _, err := svc.Book(context.Background(), 1, Income, 100, "x", "alice"); !errors.Is(err, ErrDayClosed) {
		t.Fatalf("want ErrDayClosed, got %v", err)
	}
}

func TestServiceCloseDayRejectsAlreadyClosed(t *testing.T) {
	repo := &fakeRepo{day: Day{ID: 1, Status: DayClosed}}
	svc := NewService(repo, &fakeAudit{})
	if _, err := svc.CloseDay(context.Background(), 1, "bob"); !errors.Is(err, ErrDayClosed) {
		t.Fatalf("want ErrDayClosed, got %v", err)
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("boom")
	svc := NewService(&fakeRepo{err: boom}, &fakeAudit{})
	if _, err := svc.Book(context.Background(), 1, Income, 100, "x", "a"); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if _, err := svc.OpenDay(context.Background(), time.Now(), 0, "a"); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if _, err := svc.CloseDay(context.Background(), 1, "a"); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestServicePropagatesAuditFailures(t *testing.T) {
	boom := errors.New("audit store down")
	repo := &fakeRepo{day: Day{ID: 1, Status: DayOpen, OpeningCents: 0}}
	svc := NewService(repo, failingAudit{err: boom})

	if _, err := svc.OpenDay(context.Background(), time.Now(), 0, "a"); !errors.Is(err, boom) {
		t.Fatalf("OpenDay: want the audit error, got %v", err)
	}
	if _, err := svc.Book(context.Background(), 1, Income, 100, "x", "a"); !errors.Is(err, boom) {
		t.Fatalf("Book: want the audit error, got %v", err)
	}
	if _, err := svc.CloseDay(context.Background(), 1, "a"); !errors.Is(err, boom) {
		t.Fatalf("CloseDay: want the audit error, got %v", err)
	}
}
