package klr

import (
	"testing"
	"time"
)

func TestCostCenterOverviewDefinition(t *testing.T) {
	if CostCenterOverviewDefinition.ID != "klr-cost-center-overview" {
		t.Fatalf("id = %q", CostCenterOverviewDefinition.ID)
	}
	if CostCenterOverviewDefinition.Title != "Kostenstellenübersicht" {
		t.Fatalf("title = %q", CostCenterOverviewDefinition.Title)
	}
	// Kostenstelle, Bezeichnung, Kosten, Erlöse, Saldo.
	if len(CostCenterOverviewDefinition.Columns) != 5 {
		t.Fatalf("columns = %d, want 5", len(CostCenterOverviewDefinition.Columns))
	}
	if CostCenterOverviewDefinition.Columns[0].Key != "number" || CostCenterOverviewDefinition.Columns[4].Key != "balance" {
		t.Fatalf("columns = %+v", CostCenterOverviewDefinition.Columns)
	}
}

func TestCostCenterOverviewRows(t *testing.T) {
	at := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	lines := []CostCenterLine{
		{CostCenter: CostCenter{ID: 1, Number: "K-10", Name: "Verwaltung"},
			Report: Report{CostCenterID: 1, CostsCents: 100000, RevenuesCents: 25000}},
		{CostCenter: CostCenter{ID: 2, Number: "K-99", Name: "Bibliothek"},
			Report: Report{CostCenterID: 2, CostsCents: 5000, RevenuesCents: 10000}},
	}
	got := CostCenterOverview(at, lines)

	if got.Definition.ID != CostCenterOverviewDefinition.ID || !got.GeneratedAt.Equal(at) {
		t.Fatalf("report = %+v", got)
	}
	// One row per centre plus the totals row.
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(got.Rows), got.Rows)
	}
	// A centre that spends more than it earns carries a negative balance.
	if got.Rows[0][0] != "K-10" || got.Rows[0][1] != "Verwaltung" || got.Rows[0][2] != "1000.00" ||
		got.Rows[0][3] != "250.00" || got.Rows[0][4] != "-750.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[1][0] != "K-99" || got.Rows[1][4] != "50.00" {
		t.Fatalf("second row = %v", got.Rows[1])
	}
	// The totals row sums what the rows show.
	total := got.Rows[2]
	if total[0] != "" || total[1] != "Summe" || total[2] != "1050.00" || total[3] != "350.00" || total[4] != "-700.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestCostCenterOverviewWithoutCentres(t *testing.T) {
	// No centres is an empty list and a zero totals row, not a panic.
	got := CostCenterOverview(time.Time{}, nil)
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want the totals row only", len(got.Rows))
	}
	if got.Rows[0][1] != "Summe" || got.Rows[0][2] != "0.00" || got.Rows[0][4] != "0.00" {
		t.Fatalf("totals row = %v", got.Rows[0])
	}
}

func TestCostUnitOverviewDefinition(t *testing.T) {
	if CostUnitOverviewDefinition.ID != "klr-cost-unit-overview" {
		t.Fatalf("id = %q", CostUnitOverviewDefinition.ID)
	}
	if CostUnitOverviewDefinition.Title != "Kostenträgerübersicht" {
		t.Fatalf("title = %q", CostUnitOverviewDefinition.Title)
	}
	// Kostenträger, Bezeichnung, Kosten, Erlöse, Saldo.
	if len(CostUnitOverviewDefinition.Columns) != 5 {
		t.Fatalf("columns = %d, want 5", len(CostUnitOverviewDefinition.Columns))
	}
	if CostUnitOverviewDefinition.Columns[0].Key != "number" || CostUnitOverviewDefinition.Columns[4].Key != "balance" {
		t.Fatalf("columns = %+v", CostUnitOverviewDefinition.Columns)
	}
	// The two KLR overviews are told apart by their id, which is what a client asks for.
	if CostUnitOverviewDefinition.ID == CostCenterOverviewDefinition.ID {
		t.Fatal("both KLR overviews share an id")
	}
}

func TestCostUnitOverviewRows(t *testing.T) {
	at := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	lines := []CostUnitLine{
		{CostUnit: CostUnit{ID: 1, Number: "T-1", Name: "Studiengang A"}, CostsCents: 100000, RevenuesCents: 25000},
		{CostUnit: CostUnit{ID: 2, Number: "T-2", Name: "Studiengang B"}, CostsCents: 5000, RevenuesCents: 10000},
		// A unit nothing is booked on is a row of zeroes, not a missing row.
		{CostUnit: CostUnit{ID: 3, Number: "T-3", Name: "Studiengang C"}},
	}
	got := CostUnitOverview(at, lines)

	if got.Definition.ID != CostUnitOverviewDefinition.ID || !got.GeneratedAt.Equal(at) {
		t.Fatalf("report = %+v", got)
	}
	if len(got.Rows) != 4 {
		t.Fatalf("rows = %d, want 4 (%v)", len(got.Rows), got.Rows)
	}
	if got.Rows[0][0] != "T-1" || got.Rows[0][1] != "Studiengang A" || got.Rows[0][2] != "1000.00" ||
		got.Rows[0][3] != "250.00" || got.Rows[0][4] != "-750.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[1][4] != "50.00" {
		t.Fatalf("second row = %v", got.Rows[1])
	}
	if got.Rows[2][2] != "0.00" || got.Rows[2][3] != "0.00" || got.Rows[2][4] != "0.00" {
		t.Fatalf("unit without records = %v", got.Rows[2])
	}
	total := got.Rows[3]
	if total[0] != "" || total[1] != "Summe" || total[2] != "1050.00" || total[3] != "350.00" || total[4] != "-700.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestCostUnitOverviewWithoutUnits(t *testing.T) {
	// No units is an empty list and a zero totals row, not a panic.
	got := CostUnitOverview(time.Time{}, nil)
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %d, want the totals row only", len(got.Rows))
	}
	if got.Rows[0][1] != "Summe" || got.Rows[0][2] != "0.00" || got.Rows[0][4] != "0.00" {
		t.Fatalf("totals row = %v", got.Rows[0])
	}
}

func TestKlrCents(t *testing.T) {
	cases := map[int64]string{
		0:       "0.00",
		1:       "0.01",
		99:      "0.99",
		100:     "1.00",
		125000:  "1250.00",
		-75000:  "-750.00",
		-1:      "-0.01",
		1000000: "10000.00",
	}
	for cents, want := range cases {
		if got := klrCents(cents); got != want {
			t.Errorf("klrCents(%d) = %q, want %q", cents, got, want)
		}
	}
}
