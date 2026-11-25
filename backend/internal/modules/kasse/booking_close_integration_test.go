package kasse_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestClosingBalanceCountsEveryAcceptedBooking races bookings against the close
// of the day they belong to. The invariant the Kassenabschluss rests on is that
// a closed day's closing balance equals its opening balance plus every booking
// stored against it — a booking must never land on a day after the close counted
// it, and the close must never omit a booking that was accepted (#169).
func TestClosingBalanceCountsEveryAcceptedBooking(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewService(kasse.NewRepo(pool), audit.NewStore(pool))

	const opening = 1000
	const amount = 100
	const racers = 40

	day, err := svc.OpenDay(ctx, uniqueDate(t), opening, "alice")
	if err != nil {
		t.Fatalf("OpenDay: %v", err)
	}

	// Book and close at the same time: whichever statement wins the row lock,
	// the outcome must agree with the row counts afterwards.
	var wg sync.WaitGroup
	accepted := make([]bool, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.Book(ctx, day.ID, kasse.Income, amount, "race", "alice")
			accepted[i] = err == nil
			if err != nil && !errors.Is(err, kasse.ErrDayClosed) {
				t.Errorf("Book: want nil or ErrDayClosed, got %v", err)
			}
		}(i)
	}

	var closeErr error
	var closed kasse.Day
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		closed, closeErr = svc.CloseDay(ctx, day.ID, "bob")
	}()

	close(start)
	wg.Wait()

	if closeErr != nil {
		t.Fatalf("CloseDay: %v", closeErr)
	}
	if closed.Status != kasse.DayClosed {
		t.Fatalf("status = %q, want closed", closed.Status)
	}

	bookings, err := kasse.NewRepo(pool).Bookings(ctx, day.ID)
	if err != nil {
		t.Fatalf("Bookings: %v", err)
	}
	var sum int64
	for _, b := range bookings {
		sum += b.Signed()
	}
	if want := int64(opening) + sum; closed.ClosingCents == nil || *closed.ClosingCents != want {
		t.Fatalf("closing = %v, want %d (opening %d + %d bookings)", closed.ClosingCents, want, opening, len(bookings))
	}

	// Every racer that saw success must have a row; every racer that saw
	// ErrDayClosed must not have one.
	var winners int64
	for _, ok := range accepted {
		if ok {
			winners++
		}
	}
	if winners != int64(len(bookings)) {
		t.Fatalf("accepted bookings = %d, stored rows = %d", winners, len(bookings))
	}

	// A booking after the close is refused, whatever the earlier race did.
	if _, err := svc.Book(ctx, day.ID, kasse.Income, amount, "late", "alice"); !errors.Is(err, kasse.ErrDayClosed) {
		t.Fatalf("booking after close: want ErrDayClosed, got %v", err)
	}
}
