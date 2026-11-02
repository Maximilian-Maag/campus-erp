package kasse

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// DailyCloseDefinition is the cash book's daily-close report (docs/25).
var DailyCloseDefinition = report.Definition{
	ID:    "kasse-daily-close",
	Title: "Kassenabschluss",
	Columns: []report.Column{
		{Key: "date", Title: "Datum", Align: report.Left},
		{Key: "kind", Title: "Art", Align: report.Left},
		{Key: "amount", Title: "Betrag", Align: report.Right},
		{Key: "reference", Title: "Vorgang", Align: report.Left},
		{Key: "purpose", Title: "Zweck", Align: report.Left},
	},
}

// DailyCloseReport builds the report from the bookings of a period: one row per booking
// and a closing row with the totals of income, expense and the net.
func DailyCloseReport(generatedAt time.Time, bookings []LedgerBooking) report.Report {
	rows := make([][]string, 0, len(bookings)+1)
	var income, expense int64
	for _, b := range bookings {
		if b.Kind == Income {
			income += b.AmountCents
		} else if b.Kind == Expense {
			expense += b.AmountCents
		}
		rows = append(rows, []string{
			b.BusinessDate.Format("2006-01-02"),
			string(b.Kind),
			formatCents(b.AmountCents),
			b.Reference,
			b.Purpose,
		})
	}
	rows = append(rows, []string{
		"", "Summe", formatCents(income - expense),
		"Einnahmen: " + formatCents(income),
		"Ausgaben: " + formatCents(expense),
	})
	return report.Report{Definition: DailyCloseDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// formatCents renders minor units as a decimal string ("125000" -> "1250.00").
func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return sign + strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
