package budget

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// PositionLine is one row of the Mittelübersicht: a position together with what has been
// booked against it.
type PositionLine struct {
	Position Position
	Balance  Balance
}

// PositionOverviewDefinition is the budget's Mittelübersicht (docs/25): every position with
// what it was given and what has been bound or spent on it. It is a snapshot of the budget, so
// it takes no period.
var PositionOverviewDefinition = report.Definition{
	ID:    "budget-position-overview",
	Title: "Mittelübersicht",
	Columns: []report.Column{
		{Key: "year", Title: "Haushaltsjahr", Align: report.Left},
		{Key: "chapter", Title: "Kapitel", Align: report.Left},
		{Key: "title", Title: "Titel", Align: report.Left},
		{Key: "description", Title: "Bezeichnung", Align: report.Left},
		{Key: "allocated", Title: "Ansatz", Align: report.Right},
		{Key: "committed", Title: "Verpflichtet", Align: report.Right},
		{Key: "actual", Title: "Ist", Align: report.Right},
		{Key: "available", Title: "Verfügbar", Align: report.Right},
	},
}

// PositionOverview builds the Mittelübersicht: one row per position, then a totals row. The
// available column is derived per position rather than summed from a stored figure, so the
// report cannot disagree with the balance the guard uses.
func PositionOverview(generatedAt time.Time, lines []PositionLine) report.Report {
	rows := make([][]string, 0, len(lines)+1)
	allocated, committed, actual, available := int64(0), int64(0), int64(0), int64(0)
	for _, l := range lines {
		allocated += l.Balance.AllocatedCents
		committed += l.Balance.CommittedCents
		actual += l.Balance.ActualCents
		available += l.Balance.AvailableCents()
		rows = append(rows, []string{
			strconv.Itoa(l.Position.Year),
			l.Position.Chapter,
			l.Position.Title,
			l.Position.Description,
			budgetCents(l.Balance.AllocatedCents),
			budgetCents(l.Balance.CommittedCents),
			budgetCents(l.Balance.ActualCents),
			budgetCents(l.Balance.AvailableCents()),
		})
	}
	rows = append(rows, []string{
		"", "", "", "Summe",
		budgetCents(allocated),
		budgetCents(committed),
		budgetCents(actual),
		budgetCents(available),
	})
	return report.Report{Definition: PositionOverviewDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// budgetCents renders minor units as a decimal string ("125000" -> "1250.00"). Formatting
// belongs to the domain that knows the meaning (docs/25), so each domain carries its own.
func budgetCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return sign + strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
