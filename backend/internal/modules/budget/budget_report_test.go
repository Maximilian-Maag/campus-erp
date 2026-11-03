package budget

import (
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// TestPositionOverview builds the Mittelübersicht: one row per position, a totals row, and the
// available column derived per position so the report cannot disagree with the guard's balance.
func TestPositionOverview(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	got := PositionOverview(at, []PositionLine{
		{Position: Position{ID: 1, Year: 2026, Chapter: "0710", Title: "6810", Description: "Beschaffung"},
			Balance: Balance{PositionID: 1, AllocatedCents: 100000, CommittedCents: 25000, ActualCents: 15000}},
		{Position: Position{ID: 2, Year: 2026, Chapter: "0710", Title: "6820", Description: "Dienstreisen"},
			Balance: Balance{PositionID: 2, AllocatedCents: 50000}},
	})

	if got.Definition.ID != PositionOverviewDefinition.ID || !got.GeneratedAt.Equal(at) {
		t.Fatalf("report header = %+v", got)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(got.Rows))
	}
	first := got.Rows[0]
	want := []string{"2026", "0710", "6810", "Beschaffung", "1000.00", "250.00", "150.00", "600.00"}
	for i := range want {
		if first[i] != want[i] {
			t.Fatalf("first row = %v, want %v", first, want)
		}
	}
	// A position with nothing booked is fully available.
	if got.Rows[1][7] != "500.00" {
		t.Fatalf("untouched position available = %s", got.Rows[1][7])
	}
	total := got.Rows[2]
	wantTotal := []string{"", "", "", "Summe", "1500.00", "250.00", "150.00", "1100.00"}
	for i := range wantTotal {
		if total[i] != wantTotal[i] {
			t.Fatalf("totals row = %v, want %v", total, wantTotal)
		}
	}
	// Every row matches the column count, so the report validates.
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// An empty overview is still a valid report with just its totals row.
	empty := PositionOverview(at, nil)
	if len(empty.Rows) != 1 || empty.Rows[0][3] != "Summe" || empty.Rows[0][7] != "0.00" {
		t.Fatalf("empty overview = %v", empty.Rows)
	}
	if err := empty.Validate(); err != nil {
		t.Fatalf("empty Validate: %v", err)
	}
}

// TestBudgetCents: the domain formats its own amounts, including negatives and the cents.
func TestBudgetCents(t *testing.T) {
	cases := map[int64]string{0: "0.00", 5: "0.05", 100: "1.00", 125000: "1250.00", -250: "-2.50"}
	for cents, want := range cases {
		if got := budgetCents(cents); got != want {
			t.Fatalf("budgetCents(%d) = %q, want %q", cents, got, want)
		}
	}
}

// TestPositionOverviewDefinitionIsAReport: the definition has to pass the core's own validation,
// which is what the render job relies on.
func TestPositionOverviewDefinitionIsAReport(t *testing.T) {
	if err := PositionOverviewDefinition.Validate(); err != nil {
		t.Fatalf("definition: %v", err)
	}
	cols := PositionOverviewDefinition.Columns
	if len(cols) != 8 || cols[7].Key != "available" || cols[4].Key != "allocated" {
		t.Fatalf("columns = %+v", cols)
	}
	if cols[4].Align != report.Right || cols[0].Align != report.Left {
		t.Fatalf("alignment = %v / %v", cols[0].Align, cols[4].Align)
	}
}
