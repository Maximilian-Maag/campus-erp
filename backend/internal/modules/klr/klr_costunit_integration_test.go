package klr_test

import (
	"context"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// TestCostUnitFigures: the Kostenträger view. Every unit is listed — one nothing is booked on
// carries zeroes rather than disappearing — and a record's cost unit is what puts it there.
func TestCostUnitFigures(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	center, err := svc.CreateCostCenter(ctx, "K-"+mark, "Verwaltung", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	withRecords, err := svc.CreateCostUnit(ctx, "T1-"+mark, "Studiengang A", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}
	// A unit nothing is booked on. It has to appear in the report all the same.
	empty, err := svc.CreateCostUnit(ctx, "T2-"+mark, "Studiengang B", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}

	if _, err := svc.Post(ctx, center.ID, &withRecords.ID, nil, klr.Cost, 100000, klrDay(2026, 4, 10), "RE-1-"+mark, "alice"); err != nil {
		t.Fatalf("Post cost: %v", err)
	}
	if _, err := svc.Post(ctx, center.ID, &withRecords.ID, nil, klr.Cost, 5000, klrDay(2026, 4, 11), "RE-2-"+mark, "alice"); err != nil {
		t.Fatalf("Post cost: %v", err)
	}
	if _, err := svc.Post(ctx, center.ID, &withRecords.ID, nil, klr.Revenue, 25000, klrDay(2026, 4, 12), "RE-3-"+mark, "alice"); err != nil {
		t.Fatalf("Post revenue: %v", err)
	}
	// A record with no cost unit belongs to no unit and must not land in one by accident.
	if _, err := svc.Post(ctx, center.ID, nil, nil, klr.Cost, 999999, klrDay(2026, 4, 13), "RE-4-"+mark, "alice"); err != nil {
		t.Fatalf("Post without a unit: %v", err)
	}

	lines, err := svc.CostUnitFigures(ctx)
	if err != nil {
		t.Fatalf("CostUnitFigures: %v", err)
	}

	var got *klr.CostUnitLine
	var gotEmpty *klr.CostUnitLine
	for i := range lines {
		switch lines[i].CostUnit.ID {
		case withRecords.ID:
			got = &lines[i]
		case empty.ID:
			gotEmpty = &lines[i]
		}
	}
	if got == nil {
		t.Fatalf("the unit with records is missing from %d lines", len(lines))
	}
	if got.CostsCents != 105000 || got.RevenuesCents != 25000 {
		t.Fatalf("figures = costs %d, revenues %d; want 105000 and 25000", got.CostsCents, got.RevenuesCents)
	}
	if got.CostUnit.Number != "T1-"+mark || got.CostUnit.Name != "Studiengang A" {
		t.Fatalf("unit = %+v", got.CostUnit)
	}
	if gotEmpty == nil {
		t.Fatal("a cost unit without records is missing from the report")
	}
	if gotEmpty.CostsCents != 0 || gotEmpty.RevenuesCents != 0 {
		t.Fatalf("empty unit figures = %d, %d; want zeroes", gotEmpty.CostsCents, gotEmpty.RevenuesCents)
	}
	// The 999999 booked without a unit is nowhere in the per-unit figures.
	for _, l := range lines {
		if l.CostsCents > 900000 {
			t.Fatalf("a record with no cost unit was counted on unit %d", l.CostUnit.ID)
		}
	}
}
