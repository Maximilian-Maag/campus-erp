package klr

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// CostCenterOverviewDefinition is the KLR's per-centre overview (docs/25): every cost centre
// with what it carries and what it is up or down. Like the Mittelübersicht it is a snapshot of
// the books, so it takes no period.
var CostCenterOverviewDefinition = report.Definition{
	ID:    "klr-cost-center-overview",
	Title: "Kostenstellenübersicht",
	Columns: []report.Column{
		{Key: "number", Title: "Kostenstelle", Align: report.Left},
		{Key: "name", Title: "Bezeichnung", Align: report.Left},
		{Key: "costs", Title: "Kosten", Align: report.Right},
		{Key: "revenues", Title: "Erlöse", Align: report.Right},
		{Key: "balance", Title: "Saldo", Align: report.Right},
	},
}

// CostCenterLine is one cost centre with its report.
type CostCenterLine struct {
	CostCenter CostCenter
	Report     Report
}

// CostCenterOverview builds the per-centre overview: one row per centre, then a totals row. The
// balance is derived per centre from the same figures the report shows, so the totals cannot
// disagree with the rows above them.
func CostCenterOverview(generatedAt time.Time, lines []CostCenterLine) report.Report {
	rows := make([][]string, 0, len(lines)+1)
	var costs, revenues int64
	for _, l := range lines {
		costs += l.Report.CostsCents
		revenues += l.Report.RevenuesCents
		rows = append(rows, []string{
			l.CostCenter.Number,
			l.CostCenter.Name,
			klrCents(l.Report.CostsCents),
			klrCents(l.Report.RevenuesCents),
			klrCents(l.Report.BalanceCents()),
		})
	}
	rows = append(rows, []string{
		"", "Summe",
		klrCents(costs),
		klrCents(revenues),
		klrCents(revenues - costs),
	})
	return report.Report{Definition: CostCenterOverviewDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// klrCents renders minor units as a decimal string ("125000" -> "1250.00"). Formatting belongs
// to the domain that knows the meaning (docs/25), so each domain carries its own.
func klrCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return sign + strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
