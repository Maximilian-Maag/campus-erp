package kasse_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// carryingFoundation creates one year's foundation record with the carry-over flag set as asked.
func carryingFoundation(t *testing.T, tax *kasse.TaxService, year int, number string, carry bool) kasse.Foundation {
	t.Helper()
	f, err := kasse.NewFoundation(year, number, "Stiftung", carry)
	if err != nil {
		t.Fatalf("NewFoundation: %v", err)
	}
	saved, err := tax.CreateFoundation(context.Background(), f, "alice")
	if err != nil {
		t.Fatalf("CreateFoundation: %v", err)
	}
	return saved
}

// TestFoundationCarryOver is the acceptance test for closing a year: the net of one year's book
// becomes the opening movement of the same foundation's book in the next year, and the source book
// is left exactly as it was.
func TestFoundationCarryOver(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	number := "STIFT-" + uniqueSuffix(t)
	source := carryingFoundation(t, tax, 2026, number, true)
	// The next year's record is master data the operator opens; the carry-over does not invent it.
	carryingFoundation(t, tax, 2027, number, true)

	book, err := books.CreateBook(ctx, source.ID, "JAHR-2026", "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 10000, kasse.Income, "Spende", "alice"); err != nil {
		t.Fatalf("income: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 3000, kasse.Expense, "Auszahlung", "alice"); err != nil {
		t.Fatalf("expense: %v", err)
	}

	res, err := books.CarryOver(ctx, book.ID, "", "alice")
	if err != nil {
		t.Fatalf("CarryOver: %v", err)
	}
	if res.Cents != 7000 {
		t.Fatalf("carried %d cents, want 7000", res.Cents)
	}
	if res.Year != 2026 {
		t.Fatalf("closed year = %d, want 2026", res.Year)
	}
	if res.Target.ID == book.ID {
		t.Fatal("the target book must be the next year's, not the source")
	}
	if res.Target.Reference != "JAHR-2026" {
		t.Fatalf("target reference = %q, want the source's when none is given", res.Target.Reference)
	}
	if res.Booking.Kind != kasse.Income || res.Booking.AmountCents != 7000 {
		t.Fatalf("opening movement = %s %d, want income 7000", res.Booking.Kind, res.Booking.AmountCents)
	}
	if !strings.Contains(res.Booking.Purpose, "Übertrag aus 2026") {
		t.Fatalf("purpose = %q, want it to name the year it came from", res.Booking.Purpose)
	}

	// The next year's book opens with the net, and the source book is untouched.
	target, err := books.Balance(ctx, res.Target.ID)
	if err != nil {
		t.Fatalf("target balance: %v", err)
	}
	if target.Net() != 7000 {
		t.Fatalf("target net = %d, want 7000", target.Net())
	}
	after, err := books.Balance(ctx, book.ID)
	if err != nil {
		t.Fatalf("source balance: %v", err)
	}
	if after.Net() != 7000 {
		t.Fatalf("source net = %d, want it unchanged at 7000", after.Net())
	}

	// A second carry-over of the same year is refused: the money is already there.
	if _, err := books.CarryOver(ctx, book.ID, "", "alice"); !errors.Is(err, kasse.ErrCarryOverTargetNotEmpty) {
		t.Fatalf("second carry-over = %v, want ErrCarryOverTargetNotEmpty", err)
	}
}

// TestFoundationCarryOverRefusals covers the four ways a carry-over is refused, each for its own
// reason, so a caller can tell them apart.
func TestFoundationCarryOverRefusals(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	t.Run("the foundation does not carry over", func(t *testing.T) {
		f := carryingFoundation(t, tax, 2026, "STIFT-"+uniqueSuffix(t), false)
		book, err := books.CreateBook(ctx, f.ID, "JAHR", "EUR", "alice")
		if err != nil {
			t.Fatalf("CreateBook: %v", err)
		}
		if _, err := books.Book(ctx, book.ID, 5000, kasse.Income, "Spende", "alice"); err != nil {
			t.Fatalf("income: %v", err)
		}
		if _, err := books.CarryOver(ctx, book.ID, "", "alice"); !errors.Is(err, kasse.ErrCarryOverNotAllowed) {
			t.Fatalf("err = %v, want ErrCarryOverNotAllowed", err)
		}
	})

	t.Run("nothing to carry", func(t *testing.T) {
		number := "STIFT-" + uniqueSuffix(t)
		f := carryingFoundation(t, tax, 2026, number, true)
		carryingFoundation(t, tax, 2027, number, true)
		book, err := books.CreateBook(ctx, f.ID, "JAHR", "EUR", "alice")
		if err != nil {
			t.Fatalf("CreateBook: %v", err)
		}
		// Income and expense that cancel out: a net of zero carries nothing.
		if _, err := books.Book(ctx, book.ID, 5000, kasse.Income, "Spende", "alice"); err != nil {
			t.Fatalf("income: %v", err)
		}
		if _, err := books.Book(ctx, book.ID, 5000, kasse.Expense, "Auszahlung", "alice"); err != nil {
			t.Fatalf("expense: %v", err)
		}
		if _, err := books.CarryOver(ctx, book.ID, "", "alice"); !errors.Is(err, kasse.ErrCarryOverNothing) {
			t.Fatalf("err = %v, want ErrCarryOverNothing", err)
		}
	})

	t.Run("the next year is not open", func(t *testing.T) {
		f := carryingFoundation(t, tax, 2026, "STIFT-"+uniqueSuffix(t), true)
		book, err := books.CreateBook(ctx, f.ID, "JAHR", "EUR", "alice")
		if err != nil {
			t.Fatalf("CreateBook: %v", err)
		}
		if _, err := books.Book(ctx, book.ID, 5000, kasse.Income, "Spende", "alice"); err != nil {
			t.Fatalf("income: %v", err)
		}
		if _, err := books.CarryOver(ctx, book.ID, "", "alice"); !errors.Is(err, kasse.ErrCarryOverTargetMissing) {
			t.Fatalf("err = %v, want ErrCarryOverTargetMissing", err)
		}
	})

	t.Run("a deficit opens the next year as an expense", func(t *testing.T) {
		number := "STIFT-" + uniqueSuffix(t)
		f := carryingFoundation(t, tax, 2026, number, true)
		carryingFoundation(t, tax, 2027, number, true)
		book, err := books.CreateBook(ctx, f.ID, "JAHR", "EUR", "alice")
		if err != nil {
			t.Fatalf("CreateBook: %v", err)
		}
		if _, err := books.Book(ctx, book.ID, 2000, kasse.Income, "Spende", "alice"); err != nil {
			t.Fatalf("income: %v", err)
		}
		if _, err := books.Book(ctx, book.ID, 5000, kasse.Expense, "Auszahlung", "alice"); err != nil {
			t.Fatalf("expense: %v", err)
		}
		res, err := books.CarryOver(ctx, book.ID, "JAHR-2027", "alice")
		if err != nil {
			t.Fatalf("CarryOver: %v", err)
		}
		if res.Cents != -3000 {
			t.Fatalf("carried %d cents, want -3000", res.Cents)
		}
		if res.Booking.Kind != kasse.Expense || res.Booking.AmountCents != 3000 {
			t.Fatalf("opening movement = %s %d, want expense 3000", res.Booking.Kind, res.Booking.AmountCents)
		}
		if res.Target.Reference != "JAHR-2027" {
			t.Fatalf("target reference = %q, want the one asked for", res.Target.Reference)
		}
	})
}
