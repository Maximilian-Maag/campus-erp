// Package reports is the report catalogue: which reports this system renders, and how each one
// collects its data (docs/25).
//
// It exists so the worker that renders a report and the API that offers it cannot disagree
// about what exists: both read this one list. Adding a report is one entry here plus the
// definition and builder in the owning domain.
package reports

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// ErrUnknownReport is returned for an id the catalogue does not hold.
var ErrUnknownReport = errors.New("reports: unknown report id")

// ErrNoSource is returned when a report's data source was not wired. Collecting from nothing
// would produce an empty report that looks like a real one, so it is refused.
var ErrNoSource = errors.New("reports: the report's data source is not configured")

// Bookings lists the cash book's bookings for a period (kasse.LedgerRepo satisfies it).
type Bookings interface {
	ListBookingsForExport(ctx context.Context, from, to time.Time) ([]kasse.LedgerBooking, error)
}

// Positions lists budget positions and reads their balances (budget.PositionService satisfies
// it).
type Positions interface {
	Positions(ctx context.Context) ([]budget.Position, error)
	Balance(ctx context.Context, positionID int64) (budget.Balance, error)
}

// Centres lists cost centres and reads their reports (klr.Service satisfies it).
type Centres interface {
	CostCenters(ctx context.Context) ([]klr.CostCenter, error)
	Report(ctx context.Context, costCenterID int64) (klr.Report, error)
}

// CostUnits lists cost units with what is booked on them (klr.Service satisfies it).
type CostUnits interface {
	CostUnitFigures(ctx context.Context) ([]klr.CostUnitLine, error)
}

// CostTypes lists the cost types with their bridged accounts and what the books carry on them
// (klr.Service satisfies it) — the Kostenarten print view (docs/25).
type CostTypes interface {
	CostTypeFigures(ctx context.Context) ([]klr.CostTypeLine, error)
}

// Prices lists the internal prices with the cost types they price (klr.Service satisfies it) —
// the Verrechnungspreise print view (docs/25).
type Prices interface {
	ServiceRates(ctx context.Context) ([]klr.ServiceRate, error)
	CostTypes(ctx context.Context) ([]klr.CostType, error)
}

// Rules lists the apportionment rules with the centres they name (klr.Service satisfies it) —
// the Umlageregeln print view (docs/25).
type Rules interface {
	AllocationRules(ctx context.Context) ([]klr.Allocation, error)
	CostCenters(ctx context.Context) ([]klr.CostCenter, error)
}

// DataSources is the read-only data the catalogue's reports collect from.
type DataSources struct {
	Bookings  Bookings
	Positions Positions
	Centres   Centres
	Units     CostUnits
	CostTypes CostTypes
	Prices    Prices
	Rules     Rules
}

// Report is one catalogue entry: the definition a client asks for, and the collector that fills
// it.
type Report struct {
	Definition report.Definition
	Collect    func(ctx context.Context, from, to time.Time) (report.Report, error)
}

// Catalogue is the list of reports this system renders.
type Catalogue struct{ reports []Report }

// NewCatalogue wires the reports, in the order a client should be offered them.
func NewCatalogue(src DataSources) *Catalogue {
	return &Catalogue{reports: []Report{
		{
			Definition: kasse.DailyCloseDefinition,
			Collect:    collectDailyClose(src.Bookings),
		},
		{
			Definition: budget.PositionOverviewDefinition,
			Collect:    collectPositionOverview(src.Positions),
		},
		{
			Definition: klr.CostCenterOverviewDefinition,
			Collect:    collectCostCenterOverview(src.Centres),
		},
		{
			Definition: klr.CostUnitOverviewDefinition,
			Collect:    collectCostUnitOverview(src.Units),
		},
		{
			Definition: klr.CostTypeOverviewDefinition,
			Collect:    collectCostTypeOverview(src.CostTypes),
		},
		{
			Definition: klr.ServiceRateOverviewDefinition,
			Collect:    collectServiceRateOverview(src.Prices),
		},
		{
			Definition: klr.AllocationOverviewDefinition,
			Collect:    collectAllocationOverview(src.Rules),
		},
	}}
}

// Definitions returns every report's definition, in catalogue order. The API serves this.
func (c *Catalogue) Definitions() []report.Definition {
	out := make([]report.Definition, 0, len(c.reports))
	for _, r := range c.reports {
		out = append(out, r.Definition)
	}
	return out
}

// IDs returns every report id, in catalogue order.
func (c *Catalogue) IDs() []string {
	out := make([]string, 0, len(c.reports))
	for _, r := range c.reports {
		out = append(out, r.Definition.ID)
	}
	return out
}

// Render collects the report with the given id. An id that is not in the catalogue is refused
// with ErrUnknownReport rather than rendering something else.
func (c *Catalogue) Render(ctx context.Context, id string, from, to time.Time) (report.Report, error) {
	for _, r := range c.reports {
		if r.Definition.ID == id {
			return r.Collect(ctx, from, to)
		}
	}
	return report.Report{}, fmt.Errorf("%w: %q", ErrUnknownReport, id)
}

// collectDailyClose builds the cash book's daily-close report from a period's bookings.
func collectDailyClose(bookings Bookings) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, from, to time.Time) (report.Report, error) {
		if bookings == nil {
			return report.Report{}, ErrNoSource
		}
		list, err := bookings.ListBookingsForExport(ctx, from, to)
		if err != nil {
			return report.Report{}, err
		}
		return kasse.DailyCloseReport(generatedAt(), list), nil
	}
}

// collectPositionOverview builds the Mittelübersicht: every position with the balance booked
// against it.
func collectPositionOverview(positions Positions) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if positions == nil {
			return report.Report{}, ErrNoSource
		}
		list, err := positions.Positions(ctx)
		if err != nil {
			return report.Report{}, err
		}
		lines := make([]budget.PositionLine, 0, len(list))
		for _, p := range list {
			bal, err := positions.Balance(ctx, p.ID)
			if err != nil {
				return report.Report{}, err
			}
			lines = append(lines, budget.PositionLine{Position: p, Balance: bal})
		}
		return budget.PositionOverview(generatedAt(), lines), nil
	}
}

// collectCostCenterOverview builds the per-centre overview: every cost centre with the report
// booked against it.
func collectCostCenterOverview(centres Centres) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if centres == nil {
			return report.Report{}, ErrNoSource
		}
		list, err := centres.CostCenters(ctx)
		if err != nil {
			return report.Report{}, err
		}
		lines := make([]klr.CostCenterLine, 0, len(list))
		for _, c := range list {
			rep, err := centres.Report(ctx, c.ID)
			if err != nil {
				return report.Report{}, err
			}
			lines = append(lines, klr.CostCenterLine{CostCenter: c, Report: rep})
		}
		return klr.CostCenterOverview(generatedAt(), lines), nil
	}
}

// collectCostUnitOverview builds the per-cost-unit overview (Kostenträgerübersicht). The figures
// come from one query rather than one per unit: a unit the books have never seen still belongs in
// the report, and the source decides that, not this loop.
func collectCostUnitOverview(units CostUnits) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if units == nil {
			return report.Report{}, ErrNoSource
		}
		lines, err := units.CostUnitFigures(ctx)
		if err != nil {
			return report.Report{}, err
		}
		return klr.CostUnitOverview(generatedAt(), lines), nil
	}
}

// collectCostTypeOverview builds the Kostenartenübersicht from the cost types with their accounts
// and what the books carry on them.
func collectCostTypeOverview(costTypes CostTypes) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if costTypes == nil {
			return report.Report{}, ErrNoSource
		}
		lines, err := costTypes.CostTypeFigures(ctx)
		if err != nil {
			return report.Report{}, err
		}
		return klr.CostTypeOverview(generatedAt(), lines), nil
	}
}

// collectServiceRateOverview builds the Verrechnungspreise view. The prices name a cost type by
// id, so the collector resolves the code and the name — the print view states which service a
// price belongs to, not which row it is. The rows are ordered by cost type and then by validity,
// so a price change reads as a column rather than as an accident of insertion order.
func collectServiceRateOverview(prices Prices) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if prices == nil {
			return report.Report{}, ErrNoSource
		}
		rates, err := prices.ServiceRates(ctx)
		if err != nil {
			return report.Report{}, err
		}
		types, err := prices.CostTypes(ctx)
		if err != nil {
			return report.Report{}, err
		}
		byID := make(map[int64]klr.CostType, len(types))
		for _, t := range types {
			byID[t.ID] = t
		}
		lines := make([]klr.ServiceRateLine, 0, len(rates))
		for _, r := range rates {
			t := byID[r.CostTypeID]
			lines = append(lines, klr.ServiceRateLine{Rate: r, CostTypeCode: t.Code, CostTypeName: t.Name})
		}
		sort.SliceStable(lines, func(i, j int) bool {
			if lines[i].CostTypeCode != lines[j].CostTypeCode {
				return lines[i].CostTypeCode < lines[j].CostTypeCode
			}
			return lines[i].Rate.ValidFrom.Before(lines[j].Rate.ValidFrom)
		})
		return klr.ServiceRateOverview(generatedAt(), lines), nil
	}
}

// collectAllocationOverview builds the Umlageregeln view. A rule names its two ends by id, so the
// collector resolves both — a print view that only printed ids would not be readable.
func collectAllocationOverview(rules Rules) func(context.Context, time.Time, time.Time) (report.Report, error) {
	return func(ctx context.Context, _ time.Time, _ time.Time) (report.Report, error) {
		if rules == nil {
			return report.Report{}, ErrNoSource
		}
		list, err := rules.AllocationRules(ctx)
		if err != nil {
			return report.Report{}, err
		}
		centres, err := rules.CostCenters(ctx)
		if err != nil {
			return report.Report{}, err
		}
		byID := make(map[int64]klr.CostCenter, len(centres))
		for _, c := range centres {
			byID[c.ID] = c
		}
		lines := make([]klr.AllocationLine, 0, len(list))
		for _, a := range list {
			source, target := byID[a.SourceCenterID], byID[a.TargetCenterID]
			lines = append(lines, klr.AllocationLine{
				Allocation:   a,
				SourceNumber: source.Number,
				SourceName:   source.Name,
				TargetNumber: target.Number,
				TargetName:   target.Name,
			})
		}
		return klr.AllocationOverview(generatedAt(), lines), nil
	}
}

// generatedAt is the report's timestamp; a variable so tests can pin it.
var generatedAt = func() time.Time { return time.Now().UTC() }
