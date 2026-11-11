package klr

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// The KLR print views (docs/25): the layouts an operator prints or hands on, built as report
// definitions so the one catalogue and the one renderer serve them too. They are the *rules* side
// of the KLR — which accounts a cost type stands for, what a service costs, which centre apportions
// to which — next to the two overviews, which are the *books* side (what a centre or a unit
// carries). Like those, all three are snapshots of the configuration and the books, so they take
// no period.

// CostTypeOverviewDefinition is the Kostenartenübersicht (docs/25): every cost type with the
// accounts bridged to it and what the books carry on it.
var CostTypeOverviewDefinition = report.Definition{
	ID:    "klr-cost-type-overview",
	Title: "Kostenartenübersicht",
	Columns: []report.Column{
		{Key: "code", Title: "Kostenart", Align: report.Left},
		{Key: "name", Title: "Bezeichnung", Align: report.Left},
		{Key: "accounts", Title: "Konten", Align: report.Left},
		{Key: "costs", Title: "Kosten", Align: report.Right},
		{Key: "revenues", Title: "Erlöse", Align: report.Right},
		{Key: "balance", Title: "Saldo", Align: report.Right},
	},
}

// CostTypeLine is one cost type with the accounts bridged to it and what the books carry on it.
type CostTypeLine struct {
	CostType      CostType
	Accounts      []string
	CostsCents    int64
	RevenuesCents int64
}

// CostTypeOverview builds the Kostenartenübersicht: one row per cost type, then a totals row. The
// balance is derived per type from the figures its row shows, so the totals cannot disagree with
// the rows above them.
func CostTypeOverview(generatedAt time.Time, lines []CostTypeLine) report.Report {
	rows := make([][]string, 0, len(lines)+1)
	var costs, revenues int64
	for _, l := range lines {
		costs += l.CostsCents
		revenues += l.RevenuesCents
		rows = append(rows, []string{
			l.CostType.Code,
			l.CostType.Name,
			strings.Join(l.Accounts, ", "),
			klrCents(l.CostsCents),
			klrCents(l.RevenuesCents),
			klrCents(l.RevenuesCents - l.CostsCents),
		})
	}
	rows = append(rows, []string{
		"", "Summe", "",
		klrCents(costs),
		klrCents(revenues),
		klrCents(revenues - costs),
	})
	return report.Report{Definition: CostTypeOverviewDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// ServiceRateOverviewDefinition is the Verrechnungspreise view (docs/25): what a cost type's unit
// of service costs, and from when to when that price holds.
var ServiceRateOverviewDefinition = report.Definition{
	ID:    "klr-service-rate-overview",
	Title: "Verrechnungspreise",
	Columns: []report.Column{
		{Key: "code", Title: "Kostenart", Align: report.Left},
		{Key: "name", Title: "Bezeichnung", Align: report.Left},
		{Key: "price", Title: "Preis", Align: report.Right},
		{Key: "valid_from", Title: "Gültig ab", Align: report.Left},
		{Key: "valid_to", Title: "Gültig bis", Align: report.Left},
	},
}

// ServiceRateLine is one price with the cost type it prices.
type ServiceRateLine struct {
	Rate         ServiceRate
	CostTypeCode string
	CostTypeName string
}

// ServiceRateOverview builds the Verrechnungspreise view: one row per price, oldest first, with an
// open end printed as such rather than as an empty date that reads like a mistake.
func ServiceRateOverview(generatedAt time.Time, lines []ServiceRateLine) report.Report {
	rows := make([][]string, 0, len(lines))
	for _, l := range lines {
		rows = append(rows, []string{
			l.CostTypeCode,
			l.CostTypeName,
			klrCents(l.Rate.RateCents),
			klrDay(l.Rate.ValidFrom),
			klrValidUntil(l.Rate.ValidTo),
		})
	}
	return report.Report{Definition: ServiceRateOverviewDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// AllocationOverviewDefinition is the Umlageregeln view (docs/25): which cost centre apportions
// what share of its costs to which other centre, and from when to when.
var AllocationOverviewDefinition = report.Definition{
	ID:    "klr-allocation-overview",
	Title: "Umlageregeln",
	Columns: []report.Column{
		{Key: "source", Title: "Von Kostenstelle", Align: report.Left},
		{Key: "source_name", Title: "Bezeichnung", Align: report.Left},
		{Key: "target", Title: "An Kostenstelle", Align: report.Left},
		{Key: "target_name", Title: "Bezeichnung", Align: report.Left},
		{Key: "share", Title: "Anteil", Align: report.Right},
		{Key: "valid_from", Title: "Gültig ab", Align: report.Left},
		{Key: "valid_to", Title: "Gültig bis", Align: report.Left},
	},
}

// AllocationLine is one apportionment rule with both ends resolved, so the print view names the
// centres rather than printing their ids.
type AllocationLine struct {
	Allocation   Allocation
	SourceNumber string
	SourceName   string
	TargetNumber string
	TargetName   string
}

// AllocationOverview builds the Umlageregeln view: one row per rule.
func AllocationOverview(generatedAt time.Time, lines []AllocationLine) report.Report {
	rows := make([][]string, 0, len(lines))
	for _, l := range lines {
		rows = append(rows, []string{
			l.SourceNumber,
			l.SourceName,
			l.TargetNumber,
			l.TargetName,
			klrPercent(l.Allocation.ShareBP),
			klrDay(l.Allocation.ValidFrom),
			klrValidUntil(l.Allocation.ValidTo),
		})
	}
	return report.Report{Definition: AllocationOverviewDefinition, Rows: rows, GeneratedAt: generatedAt}
}

// klrPercent renders integer basis points as a percent ("1250" -> "12,5 %"), the way the operator
// typed them. Formatting belongs to the domain that knows the meaning (docs/25), so it lives here
// with klrCents rather than in a generic formatter.
func klrPercent(basisPoints int64) string {
	sign := ""
	if basisPoints < 0 {
		sign, basisPoints = "-", -basisPoints
	}
	whole := basisPoints / 100
	rest := basisPoints % 100
	switch {
	case rest == 0:
		return sign + strconv.FormatInt(whole, 10) + " %"
	case rest%10 == 0:
		return sign + strconv.FormatInt(whole, 10) + "," + strconv.FormatInt(rest/10, 10) + " %"
	default:
		return sign + strconv.FormatInt(whole, 10) + "," + fmt.Sprintf("%02d", rest) + " %"
	}
}

// klrDay renders a date the way the other print views do.
func klrDay(day time.Time) string { return day.Format("02.01.2006") }

// klrValidUntil renders a validity's end: an open end is stated as open rather than printed as an
// empty cell that reads like a missing value.
func klrValidUntil(to *time.Time) string {
	if to == nil {
		return "offen"
	}
	return klrDay(*to)
}
