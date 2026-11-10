package klr

import (
	"testing"
	"time"
)

func printDay(d int) time.Time { return time.Date(2026, 4, d, 0, 0, 0, 0, time.UTC) }

// The Kostenartenübersicht: every cost type with its accounts, then a totals row whose balance is
// the difference of its own two sums rather than a sum of the rows' balances.
func TestCostTypeOverview(t *testing.T) {
	report := CostTypeOverview(printDay(1), []CostTypeLine{
		{CostType: CostType{Code: "4000", Name: "Personalkosten"}, Accounts: []string{"4000", "4010"},
			CostsCents: 100000, RevenuesCents: 25000},
		{CostType: CostType{Code: "4200", Name: "Material"}},
	})
	if report.Definition.ID != CostTypeOverviewDefinition.ID {
		t.Fatalf("definition = %q", report.Definition.ID)
	}
	if !report.GeneratedAt.Equal(printDay(1)) {
		t.Fatalf("generated at = %v", report.GeneratedAt)
	}
	if len(report.Rows) != 3 {
		t.Fatalf("rows = %v", report.Rows)
	}
	first := report.Rows[0]
	if first[0] != "4000" || first[1] != "Personalkosten" || first[2] != "4000, 4010" ||
		first[3] != "1000.00" || first[4] != "250.00" || first[5] != "-750.00" {
		t.Fatalf("first row = %v", first)
	}
	// A cost type with no accounts and nothing booked on it: no accounts cell, zeroes.
	if second := report.Rows[1]; second[2] != "" || second[3] != "0.00" || second[5] != "0.00" {
		t.Fatalf("second row = %v", second)
	}
	total := report.Rows[2]
	if total[0] != "" || total[1] != "Summe" || total[2] != "" ||
		total[3] != "1000.00" || total[4] != "250.00" || total[5] != "-750.00" {
		t.Fatalf("totals row = %v", total)
	}
}

// An empty catalogue of cost types still renders the definition and the totals row, so a print
// view of nothing is a valid, empty report rather than a missing one.
func TestCostTypeOverviewEmpty(t *testing.T) {
	report := CostTypeOverview(printDay(1), nil)
	if len(report.Rows) != 1 || report.Rows[0][1] != "Summe" || report.Rows[0][3] != "0.00" {
		t.Fatalf("rows = %v", report.Rows)
	}
}

// The Verrechnungspreise view names the cost type a price belongs to, and renders an open end as
// open.
func TestServiceRateOverview(t *testing.T) {
	end := printDay(30)
	report := ServiceRateOverview(printDay(1), []ServiceRateLine{
		{Rate: ServiceRate{ID: 1, CostTypeID: 1, RateCents: 1250, ValidFrom: printDay(1)},
			CostTypeCode: "4000", CostTypeName: "Personalkosten"},
		{Rate: ServiceRate{ID: 2, CostTypeID: 2, RateCents: 500, ValidFrom: printDay(2), ValidTo: &end},
			CostTypeCode: "4200", CostTypeName: "Material"},
	})
	if report.Definition.ID != ServiceRateOverviewDefinition.ID || len(report.Rows) != 2 {
		t.Fatalf("report = %+v", report)
	}
	first := report.Rows[0]
	if first[0] != "4000" || first[1] != "Personalkosten" || first[2] != "12.50" ||
		first[3] != "01.04.2026" || first[4] != "offen" {
		t.Fatalf("first row = %v", first)
	}
	if second := report.Rows[1]; second[2] != "5.00" || second[4] != "30.04.2026" {
		t.Fatalf("second row = %v", second)
	}
}

// The Umlageregeln view names both ends of a rule and prints the share as the operator typed it.
func TestAllocationOverview(t *testing.T) {
	end := printDay(30)
	report := AllocationOverview(printDay(1), []AllocationLine{
		{
			Allocation:   Allocation{ID: 1, SourceCenterID: 1, TargetCenterID: 2, ShareBP: 1250, ValidFrom: printDay(1)},
			SourceNumber: "K-10", SourceName: "Verwaltung", TargetNumber: "K-20", TargetName: "Bibliothek",
		},
		{
			Allocation:   Allocation{ID: 2, SourceCenterID: 1, TargetCenterID: 3, ShareBP: 10000, ValidFrom: printDay(1), ValidTo: &end},
			SourceNumber: "K-10", SourceName: "Verwaltung", TargetNumber: "K-30", TargetName: "Mensa",
		},
	})
	if report.Definition.ID != AllocationOverviewDefinition.ID || len(report.Rows) != 2 {
		t.Fatalf("report = %+v", report)
	}
	first := report.Rows[0]
	if first[0] != "K-10" || first[1] != "Verwaltung" || first[2] != "K-20" || first[3] != "Bibliothek" ||
		first[4] != "12,5 %" || first[5] != "01.04.2026" || first[6] != "offen" {
		t.Fatalf("first row = %v", first)
	}
	if second := report.Rows[1]; second[4] != "100 %" || second[6] != "30.04.2026" {
		t.Fatalf("second row = %v", second)
	}
}

// Basis points read back as the percent the operator typed: whole shares without a fraction, one
// decimal where one is enough, two where the digits need it, and a sign where the share is
// negative.
func TestKLRPercent(t *testing.T) {
	cases := map[int64]string{
		0:     "0 %",
		1:     "0,01 %",
		5:     "0,05 %",
		10:    "0,1 %",
		50:    "0,5 %",
		99:    "0,99 %",
		100:   "1 %",
		1250:  "12,5 %",
		10000: "100 %",
		-1250: "-12,5 %",
	}
	for bp, want := range cases {
		if got := klrPercent(bp); got != want {
			t.Errorf("klrPercent(%d) = %q, want %q", bp, got, want)
		}
	}
}

func TestKLRDayAndValidity(t *testing.T) {
	if got := klrDay(printDay(1)); got != "01.04.2026" {
		t.Fatalf("klrDay = %q", got)
	}
	if got := klrValidUntil(nil); got != "offen" {
		t.Fatalf("an open end = %q, want offen", got)
	}
	end := printDay(30)
	if got := klrValidUntil(&end); got != "30.04.2026" {
		t.Fatalf("a closed end = %q", got)
	}
}
