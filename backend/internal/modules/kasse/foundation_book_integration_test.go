package kasse_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func newFoundation(t *testing.T, tax *kasse.TaxService) kasse.Foundation {
	t.Helper()
	f, err := kasse.NewFoundation(2026, "STIFT-"+uniqueSuffix(t), "Stiftung", false)
	if err != nil {
		t.Fatalf("NewFoundation: %v", err)
	}
	saved, err := tax.CreateFoundation(context.Background(), f, "alice")
	if err != nil {
		t.Fatalf("CreateFoundation: %v", err)
	}
	return saved
}

// TestFoundationBookIsolation is the S6 acceptance test: movements booked against
// one foundation's book never appear in another's.
func TestFoundationBookIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	fa := newFoundation(t, tax)
	fb := newFoundation(t, tax)

	bookA, err := books.CreateBook(ctx, fa.ID, "BOOK-A-"+uniqueSuffix(t), "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook A: %v", err)
	}
	bookB, err := books.CreateBook(ctx, fb.ID, "BOOK-B-"+uniqueSuffix(t), "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook B: %v", err)
	}
	if bookA.ID == bookB.ID {
		t.Fatal("books must be distinct")
	}

	if _, err := books.Book(ctx, bookA.ID, 10000, kasse.Income, "Spende", "alice"); err != nil {
		t.Fatalf("book A income: %v", err)
	}
	if _, err := books.Book(ctx, bookA.ID, 3000, kasse.Expense, "Auszahlung", "alice"); err != nil {
		t.Fatalf("book A expense: %v", err)
	}
	if _, err := books.Book(ctx, bookB.ID, 50000, kasse.Income, "Spende", "alice"); err != nil {
		t.Fatalf("book B income: %v", err)
	}

	// Bookings are scoped: A sees its two, B sees its one.
	la, err := books.Bookings(ctx, bookA.ID)
	if err != nil {
		t.Fatalf("Bookings A: %v", err)
	}
	if len(la) != 2 {
		t.Fatalf("book A bookings = %d, want 2", len(la))
	}
	for _, b := range la {
		if b.BookID != bookA.ID {
			t.Fatalf("book A returned a booking of book %d", b.BookID)
		}
	}
	lb, err := books.Bookings(ctx, bookB.ID)
	if err != nil {
		t.Fatalf("Bookings B: %v", err)
	}
	if len(lb) != 1 || lb[0].AmountCents != 50000 {
		t.Fatalf("book B bookings = %+v", lb)
	}

	// Balances are independent.
	ba, err := books.Balance(ctx, bookA.ID)
	if err != nil {
		t.Fatalf("Balance A: %v", err)
	}
	if ba.IncomeCents != 10000 || ba.ExpenseCents != 3000 || ba.Net() != 7000 {
		t.Fatalf("balance A = %+v", ba)
	}
	bb, err := books.Balance(ctx, bookB.ID)
	if err != nil {
		t.Fatalf("Balance B: %v", err)
	}
	if bb.IncomeCents != 50000 || bb.ExpenseCents != 0 || bb.Net() != 50000 {
		t.Fatalf("balance B = %+v", bb)
	}

	// Audit chain intact.
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestFoundationBookValidation(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)
	f := newFoundation(t, tax)

	if _, err := books.CreateBook(ctx, f.ID, "ref", "EUR", ""); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	if _, err := books.CreateBook(ctx, 0, "ref", "EUR", "alice"); !errors.Is(err, kasse.ErrFoundationRequired) {
		t.Fatalf("no foundation: %v", err)
	}
	if _, err := books.CreateBook(ctx, f.ID, "  ", "EUR", "alice"); !errors.Is(err, kasse.ErrFoundationBookReferenceRequired) {
		t.Fatalf("no reference: %v", err)
	}
	if _, err := books.CreateBook(ctx, f.ID, "ref", "EURO", "alice"); !errors.Is(err, kasse.ErrBadCurrency) {
		t.Fatalf("bad currency: %v", err)
	}
	if _, err := books.CreateBook(ctx, 9223372036854775807, "ref-"+uniqueSuffix(t), "EUR", "alice"); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("unknown foundation: %v", err)
	}

	// Duplicate reference for the same foundation.
	ref := "DUP-" + uniqueSuffix(t)
	if _, err := books.CreateBook(ctx, f.ID, ref, "EUR", "alice"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := books.CreateBook(ctx, f.ID, ref, "EUR", "alice"); !errors.Is(err, kasse.ErrFoundationBookExists) {
		t.Fatalf("duplicate: %v", err)
	}

	// Booking validation.
	book, err := books.CreateBook(ctx, f.ID, "B-"+uniqueSuffix(t), "EUR", "alice")
	if err != nil {
		t.Fatalf("create book: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 100, kasse.Income, "p", ""); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("book no actor: %v", err)
	}
	if _, err := books.Book(ctx, 0, 100, kasse.Income, "p", "alice"); !errors.Is(err, kasse.ErrFoundationBookNotFound) {
		t.Fatalf("book no book: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 0, kasse.Income, "p", "alice"); !errors.Is(err, kasse.ErrAmountNotPositive) {
		t.Fatalf("book zero: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 100, "transfer", "p", "alice"); !errors.Is(err, kasse.ErrUnknownKind) {
		t.Fatalf("book bad kind: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 100, kasse.Income, "  ", "alice"); !errors.Is(err, kasse.ErrPurposeRequired) {
		t.Fatalf("book no purpose: %v", err)
	}
	if _, err := books.Book(ctx, 9223372036854775807, 100, kasse.Income, "p", "alice"); !errors.Is(err, kasse.ErrFoundationBookNotFound) {
		t.Fatalf("book unknown book: %v", err)
	}
}
