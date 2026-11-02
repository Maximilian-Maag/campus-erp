package kasse

import (
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

func TestDailyCloseReport(t *testing.T) {
	generated := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	bookings := []LedgerBooking{
		{Reference: "BK-1", BusinessDate: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Kind: Income, AmountCents: 125000, Purpose: "Strom"},
		{Reference: "BK-2", BusinessDate: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Kind: Expense, AmountCents: 4999, Purpose: "Buch"},
	}
	rep := DailyCloseReport(generated, bookings)
	if err := rep.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if rep.ID != DailyCloseDefinition.ID || !rep.GeneratedAt.Equal(generated) {
		t.Fatalf("report = %+v", rep)
	}
	// two bookings + the closing row
	if len(rep.Rows) != 3 {
		t.Fatalf("rows = %d", len(rep.Rows))
	}
	if got := rep.Rows[0][2]; got != "1250.00" {
		t.Fatalf("income amount = %q", got)
	}
	closing := rep.Rows[2]
	if closing[1] != "Summe" || closing[2] != "1200.01" {
		t.Fatalf("closing row = %v", closing)
	}
	if !strings.Contains(closing[3], "1250.00") || !strings.Contains(closing[4], "49.99") {
		t.Fatalf("closing row totals = %v", closing)
	}
}

func TestDailyCloseReportNegativeNet(t *testing.T) {
	rep := DailyCloseReport(time.Time{}, []LedgerBooking{
		{Reference: "BK-1", BusinessDate: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Kind: Expense, AmountCents: 500},
	})
	if got := rep.Rows[1][2]; got != "-5.00" {
		t.Fatalf("net = %q", got)
	}
	csv, err := rep.MarshalCSV()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(csv, "Datum,Art,Betrag,Vorgang,Zweck\n") {
		t.Fatalf("csv = %q", csv)
	}
	var _ report.Report = rep
}
