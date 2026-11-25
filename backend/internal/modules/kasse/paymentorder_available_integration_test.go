package kasse_test

import (
	"context"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestAvailableOrdersExcludeTheOnesARunGathered proves the picker's filter: an order that a
// payment run already gathered is not offered again, while the same order stays in the unfiltered
// list. The second run's refusal is what the filter exists to prevent (#180).
func TestAvailableOrdersExcludeTheOnesARunGathered(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)

	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	gathered := newApprovedOrder(t, orderSvc, 1200)
	stillFree := newApprovedOrder(t, orderSvc, 1300)

	// Both are available before the run: the filter must not hide an order merely because it is
	// approved.
	before, err := orderSvc.Orders(ctx, kasse.OrderApproved, true, 200, 0)
	if err != nil {
		t.Fatalf("Orders(available, before): %v", err)
	}
	if !containsOrder(before, gathered.ID) || !containsOrder(before, stillFree.ID) {
		t.Fatalf("available list before the run is missing an order: %v", ids(before))
	}

	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{gathered.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	after, err := orderSvc.Orders(ctx, kasse.OrderApproved, true, 200, 0)
	if err != nil {
		t.Fatalf("Orders(available, after): %v", err)
	}
	if containsOrder(after, gathered.ID) {
		t.Fatalf("an order a run gathered is still offered as available: %v", ids(after))
	}
	if !containsOrder(after, stillFree.ID) {
		t.Fatalf("an order no run gathered was dropped from the available list: %v", ids(after))
	}

	// The unfiltered list still carries it: the filter narrows the picker, it does not hide the
	// order from every screen.
	all, err := orderSvc.Orders(ctx, kasse.OrderApproved, false, 200, 0)
	if err != nil {
		t.Fatalf("Orders(unfiltered): %v", err)
	}
	if !containsOrder(all, gathered.ID) {
		t.Fatalf("the unfiltered list lost the gathered order: %v", ids(all))
	}
}

func containsOrder(orders []kasse.OrderSummary, id int64) bool {
	for _, o := range orders {
		if o.ID == id {
			return true
		}
	}
	return false
}

func ids(orders []kasse.OrderSummary) []int64 {
	out := make([]int64, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.ID)
	}
	return out
}
