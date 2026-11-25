package kasse_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestConcurrentCarryOversPostTheNetOnce races two carry-overs of the same year
// against each other. The check "is the target book still empty" and the insert
// used to be separate statements, so both callers could see an empty target and
// both post the net — the next year opening with double the money and every row
// looking legitimate (#171). The partial unique index on the opening movement
// makes the second one lose the race at the insert instead.
func TestConcurrentCarryOversPostTheNetOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	tax := kasse.NewTaxService(kasse.NewTaxRepo(pool), store)
	books := kasse.NewFoundationBookService(kasse.NewFoundationBookRepo(pool), store)

	number := "STIFT-" + uniqueSuffix(t)
	source := carryingFoundation(t, tax, 2026, number, true)
	carryingFoundation(t, tax, 2027, number, true)

	book, err := books.CreateBook(ctx, source.ID, "JAHR-2026", "EUR", "alice")
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if _, err := books.Book(ctx, book.ID, 10000, kasse.Income, "Spende", "alice"); err != nil {
		t.Fatalf("income: %v", err)
	}

	const racers = 2
	var wg sync.WaitGroup
	results := make([]kasse.CarryOver, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = books.CarryOver(ctx, book.ID, "", "alice")
		}(i)
	}
	close(start)
	wg.Wait()

	var succeeded int
	for i := 0; i < racers; i++ {
		switch {
		case errs[i] == nil:
			succeeded++
		case errors.Is(errs[i], kasse.ErrCarryOverTargetNotEmpty), errors.Is(errs[i], kasse.ErrFoundationBookExists):
			// The loser is refused for the right reason: the money is already there.
		default:
			t.Fatalf("carry-over %d: unexpected error %v", i, errs[i])
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful carry-overs = %d, want exactly 1", succeeded)
	}

	// The next year's book carries the net once, not twice.
	var targetID int64
	for i := 0; i < racers; i++ {
		if errs[i] == nil {
			targetID = results[i].Target.ID
		}
	}
	bal, err := books.Balance(ctx, targetID)
	if err != nil {
		t.Fatalf("target balance: %v", err)
	}
	if bal.Net() != 10000 {
		t.Fatalf("target net = %d, want 10000 (the net posted once)", bal.Net())
	}
}
