package reports_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/reports"
)

type fakeBookings struct {
	list     []kasse.LedgerBooking
	err      error
	from, to time.Time
}

func (f *fakeBookings) ListBookingsForExport(_ context.Context, from, to time.Time) ([]kasse.LedgerBooking, error) {
	f.from, f.to = from, to
	return f.list, f.err
}

type fakePositions struct {
	positions []budget.Position
	balances  map[int64]budget.Balance
	err       error
}

func (f *fakePositions) Positions(context.Context) ([]budget.Position, error) {
	return f.positions, f.err
}

func (f *fakePositions) Balance(_ context.Context, id int64) (budget.Balance, error) {
	if f.err != nil {
		return budget.Balance{}, f.err
	}
	b, ok := f.balances[id]
	if !ok {
		return budget.Balance{}, errors.New("no balance for the position")
	}
	return b, nil
}

func period() (time.Time, time.Time) {
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0)
}

type fakeCentres struct {
	centres []klr.CostCenter
	reports map[int64]klr.Report
	err     error
}

func (f *fakeCentres) CostCenters(context.Context) ([]klr.CostCenter, error) {
	return f.centres, f.err
}

func (f *fakeCentres) Report(_ context.Context, id int64) (klr.Report, error) {
	if f.err != nil {
		return klr.Report{}, f.err
	}
	r, ok := f.reports[id]
	if !ok {
		return klr.Report{}, errors.New("no report for the centre")
	}
	return r, nil
}

type fakeCostUnits struct {
	lines []klr.CostUnitLine
	err   error
}

func (f *fakeCostUnits) CostUnitFigures(context.Context) ([]klr.CostUnitLine, error) {
	return f.lines, f.err
}

type fakeCostTypes struct {
	lines []klr.CostTypeLine
	err   error
}

func (f *fakeCostTypes) CostTypeFigures(context.Context) ([]klr.CostTypeLine, error) {
	return f.lines, f.err
}

type fakePrices struct {
	rates    []klr.ServiceRate
	types    []klr.CostType
	ratesErr error
	typesErr error
}

func (f *fakePrices) ServiceRates(context.Context) ([]klr.ServiceRate, error) {
	return f.rates, f.ratesErr
}

func (f *fakePrices) CostTypes(context.Context) ([]klr.CostType, error) {
	return f.types, f.typesErr
}

type fakeRules struct {
	rules      []klr.Allocation
	centres    []klr.CostCenter
	rulesErr   error
	centresErr error
}

func (f *fakeRules) AllocationRules(context.Context) ([]klr.Allocation, error) {
	return f.rules, f.rulesErr
}

func (f *fakeRules) CostCenters(context.Context) ([]klr.CostCenter, error) {
	return f.centres, f.centresErr
}

// TestCatalogueIsTheSingleList: the catalogue's definitions are the domains' own, in catalogue
// order, so the API offering a report and the worker rendering it cannot disagree.
func TestCatalogueIsTheSingleList(t *testing.T) {
	c := reports.NewCatalogue(reports.DataSources{})

	defs := c.Definitions()
	want := []string{
		kasse.DailyCloseDefinition.ID,
		budget.PositionOverviewDefinition.ID,
		klr.CostCenterOverviewDefinition.ID,
		klr.CostUnitOverviewDefinition.ID,
		klr.CostTypeOverviewDefinition.ID,
		klr.ServiceRateOverviewDefinition.ID,
		klr.AllocationOverviewDefinition.ID,
	}
	if len(defs) != len(want) {
		t.Fatalf("definitions = %d, want %d", len(defs), len(want))
	}
	for i, id := range want {
		if defs[i].ID != id {
			t.Fatalf("definition %d = %q, want %q", i, defs[i].ID, id)
		}
	}
	if defs[1].Title != budget.PositionOverviewDefinition.Title || len(defs[1].Columns) != len(budget.PositionOverviewDefinition.Columns) {
		t.Fatalf("second definition is not the budget overview: %+v", defs[1])
	}
	if defs[2].Title != klr.CostCenterOverviewDefinition.Title || len(defs[2].Columns) != len(klr.CostCenterOverviewDefinition.Columns) {
		t.Fatalf("third definition is not the cost-centre overview: %+v", defs[2])
	}
	if defs[3].Title != klr.CostUnitOverviewDefinition.Title || len(defs[3].Columns) != len(klr.CostUnitOverviewDefinition.Columns) {
		t.Fatalf("fourth definition is not the cost-unit overview: %+v", defs[3])
	}

	ids := c.IDs()
	if len(ids) != len(defs) {
		t.Fatalf("ids = %v, want %v", ids, defs)
	}
	for i := range ids {
		if ids[i] != defs[i].ID {
			t.Fatalf("ids = %v, want the definitions' own", ids)
		}
	}

	// An empty catalogue has no definitions rather than a nil panic.
	if empty := (&reports.Catalogue{}); len(empty.Definitions()) != 0 || len(empty.IDs()) != 0 {
		t.Fatal("an empty catalogue reports definitions")
	}
}

func TestRenderDailyClose(t *testing.T) {
	from, to := period()
	bookings := &fakeBookings{list: []kasse.LedgerBooking{
		{Kind: kasse.Income, AmountCents: 5000, Reference: "R-1", Purpose: "Gebühr",
			BusinessDate: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}}
	c := reports.NewCatalogue(reports.DataSources{Bookings: bookings})

	got, err := c.Render(context.Background(), kasse.DailyCloseDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got.Definition.ID != kasse.DailyCloseDefinition.ID {
		t.Fatalf("definition = %q", got.Definition.ID)
	}
	// One row per booking plus the closing row.
	if len(got.Rows) != 2 || got.Rows[0][3] != "R-1" || got.Rows[1][1] != "Summe" {
		t.Fatalf("rows = %v", got.Rows)
	}
	if got.GeneratedAt.IsZero() {
		t.Fatal("report has no generated-at timestamp")
	}
	// The period reaches the collector's data source.
	if !bookings.from.Equal(from) || !bookings.to.Equal(to) {
		t.Fatalf("collector asked for %v..%v, want %v..%v", bookings.from, bookings.to, from, to)
	}
}

func TestRenderPositionOverview(t *testing.T) {
	from, to := period()
	positions := &fakePositions{
		positions: []budget.Position{
			{ID: 1, Year: 2026, Chapter: "0710", Title: "6810", Description: "Beschaffung"},
			{ID: 2, Year: 2026, Chapter: "0710", Title: "6820", Description: "Dienstreisen"},
		},
		balances: map[int64]budget.Balance{
			1: {PositionID: 1, AllocatedCents: 100000, CommittedCents: 25000, ActualCents: 15000},
			2: {PositionID: 2, AllocatedCents: 50000},
		},
	}
	c := reports.NewCatalogue(reports.DataSources{Positions: positions})

	got, err := c.Render(context.Background(), budget.PositionOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Two positions plus the totals row.
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(got.Rows), got.Rows)
	}
	if got.Rows[0][3] != "Beschaffung" || got.Rows[0][4] != "1000.00" || got.Rows[0][7] != "600.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	// The totals row sums the allocated and the available.
	total := got.Rows[2]
	if total[3] != "Summe" || total[4] != "1500.00" || total[5] != "250.00" || total[7] != "1100.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestRenderCostCenterOverview(t *testing.T) {
	from, to := period()
	centres := &fakeCentres{
		centres: []klr.CostCenter{
			{ID: 1, Number: "K-10", Name: "Verwaltung"},
			{ID: 2, Number: "K-99", Name: "Bibliothek"},
		},
		reports: map[int64]klr.Report{
			1: {CostCenterID: 1, CostsCents: 100000, RevenuesCents: 25000},
			2: {CostCenterID: 2, CostsCents: 5000, RevenuesCents: 10000},
		},
	}
	c := reports.NewCatalogue(reports.DataSources{Centres: centres})

	got, err := c.Render(context.Background(), klr.CostCenterOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Two centres plus the totals row.
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(got.Rows), got.Rows)
	}
	// Costs, revenues and the balance derived from them; a centre that spends more is negative.
	if got.Rows[0][0] != "K-10" || got.Rows[0][1] != "Verwaltung" || got.Rows[0][2] != "1000.00" ||
		got.Rows[0][3] != "250.00" || got.Rows[0][4] != "-750.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	total := got.Rows[2]
	if total[1] != "Summe" || total[2] != "1050.00" || total[3] != "350.00" || total[4] != "-700.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestRenderCostUnitOverview(t *testing.T) {
	from, to := period()
	units := &fakeCostUnits{lines: []klr.CostUnitLine{
		{CostUnit: klr.CostUnit{ID: 1, Number: "T-1", Name: "Studiengang A"}, CostsCents: 100000, RevenuesCents: 25000},
		{CostUnit: klr.CostUnit{ID: 2, Number: "T-2", Name: "Studiengang B"}, CostsCents: 5000, RevenuesCents: 10000},
		// A unit the books have never seen is still a row.
		{CostUnit: klr.CostUnit{ID: 3, Number: "T-3", Name: "Studiengang C"}},
	}}
	c := reports.NewCatalogue(reports.DataSources{Units: units})

	got, err := c.Render(context.Background(), klr.CostUnitOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Three units plus the totals row.
	if len(got.Rows) != 4 {
		t.Fatalf("rows = %d, want 4 (%v)", len(got.Rows), got.Rows)
	}
	if got.Rows[0][0] != "T-1" || got.Rows[0][1] != "Studiengang A" || got.Rows[0][2] != "1000.00" ||
		got.Rows[0][3] != "250.00" || got.Rows[0][4] != "-750.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[2][2] != "0.00" || got.Rows[2][4] != "0.00" {
		t.Fatalf("unit without records = %v", got.Rows[2])
	}
	total := got.Rows[3]
	if total[1] != "Summe" || total[2] != "1050.00" || total[3] != "350.00" || total[4] != "-700.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestRenderCostTypeOverview(t *testing.T) {
	from, to := period()
	types := &fakeCostTypes{lines: []klr.CostTypeLine{
		{
			CostType: klr.CostType{ID: 1, Code: "4000", Name: "Personalkosten"},
			Accounts: []string{"4000", "4010"}, CostsCents: 100000, RevenuesCents: 25000,
		},
		// A cost type nothing is booked on yet is still a row, with whatever accounts it owns.
		{CostType: klr.CostType{ID: 2, Code: "4200", Name: "Material"}},
	}}
	c := reports.NewCatalogue(reports.DataSources{CostTypes: types})

	got, err := c.Render(context.Background(), klr.CostTypeOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Two cost types plus the totals row.
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(got.Rows), got.Rows)
	}
	if got.Rows[0][0] != "4000" || got.Rows[0][1] != "Personalkosten" || got.Rows[0][2] != "4000, 4010" ||
		got.Rows[0][3] != "1000.00" || got.Rows[0][4] != "250.00" || got.Rows[0][5] != "-750.00" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[1][2] != "" || got.Rows[1][3] != "0.00" {
		t.Fatalf("cost type without records = %v", got.Rows[1])
	}
	total := got.Rows[2]
	if total[1] != "Summe" || total[3] != "1000.00" || total[4] != "250.00" || total[5] != "-750.00" {
		t.Fatalf("totals row = %v", total)
	}
}

func TestRenderServiceRateOverview(t *testing.T) {
	from, to := period()
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	prices := &fakePrices{
		// Deliberately out of cost-type order: the view is ordered by the cost type it prices.
		rates: []klr.ServiceRate{
			{ID: 1, CostTypeID: 2, RateCents: 500, ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 2, CostTypeID: 1, RateCents: 1250, ValidFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 3, CostTypeID: 1, RateCents: 1500, ValidFrom: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), ValidTo: &end},
		},
		types: []klr.CostType{
			{ID: 1, Code: "4000", Name: "Personalkosten"},
			{ID: 2, Code: "4200", Name: "Material"},
		},
	}
	c := reports.NewCatalogue(reports.DataSources{Prices: prices})

	got, err := c.Render(context.Background(), klr.ServiceRateOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(got.Rows), got.Rows)
	}
	// The cost type is named, and a price with no end states that rather than printing nothing.
	if got.Rows[0][0] != "4000" || got.Rows[0][1] != "Personalkosten" || got.Rows[0][2] != "12.50" ||
		got.Rows[0][3] != "01.04.2026" || got.Rows[0][4] != "offen" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[1][2] != "15.00" || got.Rows[1][4] != "30.06.2026" {
		t.Fatalf("second row = %v", got.Rows[1])
	}
	if got.Rows[2][0] != "4200" || got.Rows[2][2] != "5.00" {
		t.Fatalf("third row = %v", got.Rows[2])
	}
}

func TestRenderAllocationOverview(t *testing.T) {
	from, to := period()
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	rules := &fakeRules{
		rules: []klr.Allocation{
			{ID: 1, SourceCenterID: 1, TargetCenterID: 2, ShareBP: 1250, ValidFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
			{ID: 2, SourceCenterID: 1, TargetCenterID: 3, ShareBP: 10000, ValidFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), ValidTo: &end},
		},
		centres: []klr.CostCenter{
			{ID: 1, Number: "K-10", Name: "Verwaltung"},
			{ID: 2, Number: "K-20", Name: "Bibliothek"},
			{ID: 3, Number: "K-30", Name: "Mensa"},
		},
	}
	c := reports.NewCatalogue(reports.DataSources{Rules: rules})

	got, err := c.Render(context.Background(), klr.AllocationOverviewDefinition.ID, from, to)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// One row per rule, both ends named rather than printed as ids.
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (%v)", len(got.Rows), got.Rows)
	}
	if got.Rows[0][0] != "K-10" || got.Rows[0][1] != "Verwaltung" || got.Rows[0][2] != "K-20" ||
		got.Rows[0][3] != "Bibliothek" || got.Rows[0][4] != "12,5 %" || got.Rows[0][5] != "01.04.2026" ||
		got.Rows[0][6] != "offen" {
		t.Fatalf("first row = %v", got.Rows[0])
	}
	if got.Rows[1][4] != "100 %" || got.Rows[1][6] != "30.06.2026" {
		t.Fatalf("second row = %v", got.Rows[1])
	}
}

// TestRenderRefusals: an unknown report, a missing data source and a failing source all fail
// rather than rendering something that looks real.
func TestRenderRefusals(t *testing.T) {
	from, to := period()
	boom := errors.New("boom")

	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), "nope", from, to); !errors.Is(err, reports.ErrUnknownReport) {
		t.Fatalf("unknown id = %v, want ErrUnknownReport", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), kasse.DailyCloseDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no bookings source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), budget.PositionOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no positions source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Bookings: &fakeBookings{err: boom}}).Render(context.Background(), kasse.DailyCloseDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("bookings error = %v, want boom", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Positions: &fakePositions{err: boom}}).Render(context.Background(), budget.PositionOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("positions error = %v, want boom", err)
	}
	// A position whose balance cannot be read stops the report rather than writing a zero.
	positions := &fakePositions{positions: []budget.Position{{ID: 9, Year: 2026, Chapter: "1", Title: "2"}}}
	if _, err := reports.NewCatalogue(reports.DataSources{Positions: positions}).Render(context.Background(), budget.PositionOverviewDefinition.ID, from, to); err == nil {
		t.Fatal("a missing balance did not stop the report")
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), klr.CostCenterOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no centres source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Centres: &fakeCentres{err: boom}}).Render(context.Background(), klr.CostCenterOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("centres error = %v, want boom", err)
	}
	// A centre whose report cannot be read stops the report rather than writing a zero.
	missing := &fakeCentres{centres: []klr.CostCenter{{ID: 9, Number: "K-9"}}}
	if _, err := reports.NewCatalogue(reports.DataSources{Centres: missing}).Render(context.Background(), klr.CostCenterOverviewDefinition.ID, from, to); err == nil {
		t.Fatal("a missing centre report did not stop the report")
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), klr.CostUnitOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no cost-unit source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Units: &fakeCostUnits{err: boom}}).Render(context.Background(), klr.CostUnitOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("cost-unit figures error = %v, want boom", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), klr.CostTypeOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no cost-type source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{CostTypes: &fakeCostTypes{err: boom}}).Render(context.Background(), klr.CostTypeOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("cost-type figures error = %v, want boom", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), klr.ServiceRateOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no price source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Prices: &fakePrices{ratesErr: boom}}).Render(context.Background(), klr.ServiceRateOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("rates error = %v, want boom", err)
	}
	// The cost types a price names are read too: if they cannot be read, the view stops rather
	// than printing a price with no service.
	if _, err := reports.NewCatalogue(reports.DataSources{Prices: &fakePrices{typesErr: boom}}).Render(context.Background(), klr.ServiceRateOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("cost types error = %v, want boom", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{}).Render(context.Background(), klr.AllocationOverviewDefinition.ID, from, to); !errors.Is(err, reports.ErrNoSource) {
		t.Fatalf("no rules source = %v, want ErrNoSource", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Rules: &fakeRules{rulesErr: boom}}).Render(context.Background(), klr.AllocationOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("rules error = %v, want boom", err)
	}
	if _, err := reports.NewCatalogue(reports.DataSources{Rules: &fakeRules{centresErr: boom}}).Render(context.Background(), klr.AllocationOverviewDefinition.ID, from, to); !errors.Is(err, boom) {
		t.Fatalf("centres error = %v, want boom", err)
	}
}
