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
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
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

// DataSources is the read-only data the catalogue's reports collect from.
type DataSources struct {
	Bookings  Bookings
	Positions Positions
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

// generatedAt is the report's timestamp; a variable so tests can pin it.
var generatedAt = func() time.Time { return time.Now().UTC() }
