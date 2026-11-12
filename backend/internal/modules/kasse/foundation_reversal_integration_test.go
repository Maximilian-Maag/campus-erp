package kasse_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestFoundationBookingReversal is the acceptance test for the correction path: a movement booked
// wrongly is answered by a counter-movement beside it, the original stays where it is, and the two
// of them net the book back to where it started.
func TestFoundationBookingReversal(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	f := newFoundation(t, tax)
	book, err := books.CreateBook(ctx, f.ID, "STORNO-"+uniqueSuffix(t), "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 3000, kasse.Income, "Spende", "alice"); err != nil {
		t.Fatalf("income: %v", err)
	}
	original, err := books.Book(ctx, book.ID, 5000, kasse.Income, "Spende (doppelt)", "alice")
	if err != nil {
		t.Fatalf("second income: %v", err)
	}

	res, err := books.Reverse(ctx, original.ID, "doppelt erfasst", "alice")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if res.Cents != 5000 {
		t.Fatalf("cancelled %d cents, want 5000", res.Cents)
	}
	if res.Mirror.Kind != kasse.Expense || res.Mirror.AmountCents != 5000 {
		t.Fatalf("counter-movement = %s %d, want expense 5000", res.Mirror.Kind, res.Mirror.AmountCents)
	}
	if res.Mirror.BookID != original.BookID {
		t.Fatalf("the counter-movement belongs to book %d, want %d", res.Mirror.BookID, original.BookID)
	}
	if res.Mirror.Reverses == nil || *res.Mirror.Reverses != original.ID {
		t.Fatalf("the counter-movement does not point at %d: %+v", original.ID, res.Mirror.Reverses)
	}
	if res.Original.ReversedBy != nil {
		t.Fatal("the pre-reversal copy must not claim to be reversed")
	}
	if !strings.Contains(res.Mirror.Purpose, "Storno zu Buchung") || !strings.Contains(res.Mirror.Purpose, "doppelt erfasst") {
		t.Fatalf("purpose = %q, want it to name the movement and the reason", res.Mirror.Purpose)
	}

	// The original is marked and neither row was touched or deleted: the book carries all three.
	list, err := books.Bookings(ctx, book.ID)
	if err != nil {
		t.Fatalf("Bookings: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("the book carries %d movements, want 3 (the mistake stays)", len(list))
	}
	for _, b := range list {
		if b.ID != original.ID {
			continue
		}
		if b.ReversedBy == nil || *b.ReversedBy != res.Mirror.ID {
			t.Fatalf("the original is not marked as cancelled by %d: %+v", res.Mirror.ID, b.ReversedBy)
		}
		if b.Kind != kasse.Income || b.AmountCents != 5000 {
			t.Fatalf("the original was altered: %s %d", b.Kind, b.AmountCents)
		}
	}

	// The two of them net out: the book is back to the one movement that was right.
	bal, err := books.Balance(ctx, book.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.Net() != 3000 {
		t.Fatalf("balance = %d, want 3000 — the cancelled pair must net to zero", bal.Net())
	}
}

// TestFoundationBookingReversalRefusals covers the four ways a cancellation is refused, so a client
// can tell them apart rather than reading one generic failure.
func TestFoundationBookingReversalRefusals(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	f := newFoundation(t, tax)
	book, err := books.CreateBook(ctx, f.ID, "STORNO-"+uniqueSuffix(t), "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	expense, err := books.Book(ctx, book.ID, 2500, kasse.Expense, "Auszahlung", "alice")
	if err != nil {
		t.Fatalf("expense: %v", err)
	}

	// A deficit movement is cancelled by an income of the same amount.
	res, err := books.Reverse(ctx, expense.ID, "", "alice")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if res.Mirror.Kind != kasse.Income {
		t.Fatalf("counter-movement = %s, want income", res.Mirror.Kind)
	}
	if res.Mirror.Purpose != fmt.Sprintf("Storno zu Buchung %d", expense.ID) {
		t.Fatalf("purpose = %q, want the movement named without a reason", res.Mirror.Purpose)
	}

	// A reason typed with surrounding spaces is trimmed into the purpose rather than carried as
	// typed: the statement reads one way however the operator's form left the field.
	padded, err := books.Book(ctx, book.ID, 900, kasse.Income, "Spende (Tippfehler)", "alice")
	if err != nil {
		t.Fatalf("income: %v", err)
	}
	trimmed, err := books.Reverse(ctx, padded.ID, "   zu viel Leerzeichen   ", "alice")
	if err != nil {
		t.Fatalf("Reverse with a padded reason: %v", err)
	}
	if trimmed.Mirror.Purpose != fmt.Sprintf("Storno zu Buchung %d: zu viel Leerzeichen", padded.ID) {
		t.Fatalf("purpose = %q, want the reason trimmed", trimmed.Mirror.Purpose)
	}

	if _, err := books.Reverse(ctx, expense.ID, "", "alice"); !errors.Is(err, kasse.ErrBookingAlreadyReversed) {
		t.Fatalf("second cancellation = %v, want ErrBookingAlreadyReversed", err)
	}
	if _, err := books.Reverse(ctx, res.Mirror.ID, "", "alice"); !errors.Is(err, kasse.ErrReversalNotReversible) {
		t.Fatalf("cancelling the counter-movement = %v, want ErrReversalNotReversible", err)
	}
	if _, err := books.Reverse(ctx, 9223372036854775807, "", "alice"); !errors.Is(err, kasse.ErrBookingNotFound) {
		t.Fatalf("unknown movement = %v, want ErrBookingNotFound", err)
	}
	if _, err := books.Reverse(ctx, expense.ID, "", "  "); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
}
