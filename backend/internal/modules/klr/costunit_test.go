package klr

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewCostUnitKeyValidation(t *testing.T) {
	from := day(2026, 1, 1)
	until := day(2026, 12, 31)

	key, err := NewCostUnitKey(1, 2, 2500, from, &until)
	if err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if key.SourceCenterID != 1 || key.TargetUnitID != 2 || key.ShareBP != 2500 {
		t.Fatalf("key = %+v", key)
	}
	if key.ValidTo == nil || !key.ValidTo.Equal(until) {
		t.Fatalf("valid_to = %v", key.ValidTo)
	}

	cases := []struct {
		name                  string
		source, target, share int64
		from, to              time.Time
		toPtr                 bool
		want                  error
	}{
		{"no source centre", 0, 2, 2500, from, until, true, ErrCostCenterNotFound},
		{"negative source", -1, 2, 2500, from, until, true, ErrCostCenterNotFound},
		{"no cost unit", 1, 0, 2500, from, until, true, ErrCostUnitNotFound},
		{"share too large", 1, 2, 10001, from, until, true, ErrAllocationShare},
		{"share negative", 1, 2, -1, from, until, true, ErrAllocationShare},
		{"period backwards", 1, 2, 2500, until, from, true, ErrAllocationPeriod},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var to *time.Time
			if c.toPtr {
				to = &c.to
			}
			if _, err := NewCostUnitKey(c.source, c.target, c.share, c.from, to); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}

	// A key with no end date is fine, and 0 bp / 10000 bp are both in range.
	if _, err := NewCostUnitKey(1, 2, 0, from, nil); err != nil {
		t.Fatalf("0 bp with no end date: %v", err)
	}
	if _, err := NewCostUnitKey(1, 2, 10000, from, nil); err != nil {
		t.Fatalf("10000 bp: %v", err)
	}
}

func TestCostUnitKeyActive(t *testing.T) {
	from := day(2026, 1, 1)
	until := day(2026, 3, 31)

	open, _ := NewCostUnitKey(1, 2, 10000, from, nil)
	if open.Active(day(2025, 12, 31)) {
		t.Fatal("a key applies only from its valid_from")
	}
	if !open.Active(from) || !open.Active(day(2030, 1, 1)) {
		t.Fatal("a key with no end date is open-ended")
	}

	bounded, _ := NewCostUnitKey(1, 2, 10000, from, &until)
	if !bounded.Active(from) || !bounded.Active(until) {
		t.Fatal("the whole validity range is inclusive")
	}
	if bounded.Active(day(2026, 4, 1)) {
		t.Fatal("a key does not apply after its valid_to")
	}
}

func TestCostUnitKeyString(t *testing.T) {
	key := CostUnitKey{ID: 7, SourceCenterID: 1, TargetUnitID: 2, ShareBP: 2500}
	got := key.String()
	want := "cost unit key 7: centre 1 -> unit 2 at 2500 bp"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestSplitUnitShares(t *testing.T) {
	keys := []CostUnitKey{
		{TargetUnitID: 1, ShareBP: 3333},
		{TargetUnitID: 2, ShareBP: 3333},
		{TargetUnitID: 3, ShareBP: 3334},
	}
	shares, err := SplitUnitShares(100, keys)
	if err != nil {
		t.Fatalf("SplitUnitShares: %v", err)
	}
	if len(shares) != 3 {
		t.Fatalf("shares = %d, want 3", len(shares))
	}
	var total int64
	for _, s := range shares {
		total += s.Cents
	}
	// Every cent is handed out: the last key takes the remainder.
	if total != 100 {
		t.Fatalf("shares add up to %d, want 100", total)
	}
	if shares[2].Cents != 100-33-33 {
		t.Fatalf("last share = %d", shares[2].Cents)
	}
	if shares[0].Key.TargetUnitID != 1 || shares[1].Key.TargetUnitID != 2 {
		t.Fatalf("shares keep their keys: %+v", shares)
	}

	if _, err := SplitUnitShares(100, nil); !errors.Is(err, ErrAllocationRunNoTargets) {
		t.Fatalf("no keys = %v", err)
	}
	if _, err := SplitUnitShares(0, keys); !errors.Is(err, ErrNothingToAllocate) {
		t.Fatalf("no amount = %v", err)
	}
	incomplete := []CostUnitKey{{TargetUnitID: 1, ShareBP: 5000}}
	if _, err := SplitUnitShares(100, incomplete); !errors.Is(err, ErrSharesIncomplete) {
		t.Fatalf("shares that do not add up = %v", err)
	}
}

func TestPlanCostUnits(t *testing.T) {
	keys := []CostUnitKey{
		{TargetUnitID: 1, ShareBP: 5000},
		{TargetUnitID: 2, ShareBP: 5000},
	}
	records, err := planCostUnits(9, 1000, keys, day(2026, 4, 30), "alice")
	if err != nil {
		t.Fatalf("planCostUnits: %v", err)
	}
	// Two receiver records plus the relief on the source.
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3 (%+v)", len(records), records)
	}
	for i, want := range []int64{1, 2} {
		rec := records[i]
		if rec.CostCenterID != 9 {
			t.Fatalf("record %d sits on centre %d, want the source 9", i, rec.CostCenterID)
		}
		if rec.CostUnitID == nil || *rec.CostUnitID != want {
			t.Fatalf("record %d unit = %v, want %d", i, rec.CostUnitID, want)
		}
		if rec.Kind != Cost || rec.AmountCents != 500 || rec.BookedBy != "alice" {
			t.Fatalf("record %d = %+v", i, rec)
		}
	}
	relief := records[2]
	if relief.Kind != Revenue || relief.AmountCents != 1000 || relief.CostUnitID != nil || relief.CostCenterID != 9 {
		t.Fatalf("relief = %+v", relief)
	}
	// The records belong to the period that was distributed, not to the day the run happens.
	if !relief.BusinessDate.Equal(day(2026, 4, 30)) {
		t.Fatalf("business date = %v", relief.BusinessDate)
	}
}

func TestPlanCostUnitsSkipsSharesThatRoundToNothing(t *testing.T) {
	// 1 cent over 3333/3333/3334 bp: the first two round to nothing, so only the last books.
	keys := []CostUnitKey{
		{TargetUnitID: 1, ShareBP: 3333},
		{TargetUnitID: 2, ShareBP: 3333},
		{TargetUnitID: 3, ShareBP: 3334},
	}
	records, err := planCostUnits(9, 1, keys, day(2026, 4, 30), "alice")
	if err != nil {
		t.Fatalf("planCostUnits: %v", err)
	}
	// One receiver record and the relief, which carries the cent that was actually moved.
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2 (%+v)", len(records), records)
	}
	if records[0].CostUnitID == nil || *records[0].CostUnitID != 3 || records[0].AmountCents != 1 {
		t.Fatalf("receiver = %+v", records[0])
	}
	if records[1].Kind != Revenue || records[1].AmountCents != 1 {
		t.Fatalf("relief = %+v", records[1])
	}
}

func TestDefineCostUnitKey(t *testing.T) {
	ctx := context.Background()
	from := day(2026, 1, 1)
	repo := &fakeRepo{unit: CostUnit{ID: 2, Number: "T-2"}}
	svc := NewService(repo, okAudit{})

	saved, err := svc.DefineCostUnitKey(ctx, 1, 2, 2500, from, nil, "alice")
	if err != nil {
		t.Fatalf("DefineCostUnitKey: %v", err)
	}
	if saved.ID != 1 || saved.CreatedBy != "alice" || saved.ShareBP != 2500 {
		t.Fatalf("saved = %+v", saved)
	}

	if _, err := svc.DefineCostUnitKey(ctx, 1, 2, 2500, from, nil, "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v", err)
	}
	if _, err := svc.DefineCostUnitKey(ctx, 0, 2, 2500, from, nil, "alice"); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("no centre = %v", err)
	}
	if _, err := svc.DefineCostUnitKey(ctx, 1, 0, 2500, from, nil, "alice"); !errors.Is(err, ErrCostUnitNotFound) {
		t.Fatalf("no unit = %v", err)
	}
	// The centre is looked up before the unit, so an unknown centre reports as one.
	if _, err := NewService(&fakeRepo{err: ErrCostCenterNotFound}, okAudit{}).DefineCostUnitKey(ctx, 1, 2, 2500, from, nil, "alice"); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v", err)
	}

	boom := errors.New("boom")
	if _, err := NewService(&fakeRepo{err: boom}, okAudit{}).DefineCostUnitKey(ctx, 1, 2, 2500, from, nil, "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v", err)
	}
	if _, err := NewService(&fakeRepo{unit: CostUnit{ID: 2}}, failingAudit{err: boom}).DefineCostUnitKey(ctx, 1, 2, 2500, from, nil, "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v", err)
	}
}

func TestCostUnitsList(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{units: []CostUnit{{ID: 1, Number: "T-1"}}}
	svc := NewService(repo, okAudit{})

	units, err := svc.CostUnits(ctx)
	if err != nil || len(units) != 1 || units[0].Number != "T-1" {
		t.Fatalf("CostUnits = %+v, %v", units, err)
	}

	boom := errors.New("boom")
	if _, err := NewService(&fakeRepo{err: boom}, okAudit{}).CostUnits(ctx); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v, want boom", err)
	}
}

func TestCostUnitKeys(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{unitKeys: []CostUnitKey{{ID: 1, TargetUnitID: 2, ShareBP: 10000}}}
	svc := NewService(repo, okAudit{})

	keys, err := svc.CostUnitKeys(ctx, 1)
	if err != nil || len(keys) != 1 || keys[0].TargetUnitID != 2 {
		t.Fatalf("CostUnitKeys = %+v, %v", keys, err)
	}

	// An unknown centre is a not-found, not an empty list.
	if _, err := NewService(&fakeRepo{err: ErrCostCenterNotFound}, okAudit{}).CostUnitKeys(ctx, 1); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v", err)
	}
}

func TestAllocateToCostUnits(t *testing.T) {
	ctx := context.Background()
	from, to := day(2026, 4, 1), day(2026, 4, 30)
	keys := []CostUnitKey{{ID: 1, TargetUnitID: 2, ShareBP: 10000, ValidFrom: day(2026, 1, 1)}}
	repo := &fakeRepo{unitKeys: keys, costs: 5000}
	svc := NewService(repo, okAudit{})

	run, err := svc.AllocateToCostUnits(ctx, 1, from, to, "alice")
	if err != nil {
		t.Fatalf("AllocateToCostUnits: %v", err)
	}
	if run.ID != 1 || run.AllocatedCents != 5000 || run.Receivers != 1 || run.RunBy != "alice" {
		t.Fatalf("run = %+v", run)
	}

	if _, err := svc.AllocateToCostUnits(ctx, 1, from, to, " "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v", err)
	}
	if _, err := svc.AllocateToCostUnits(ctx, 1, to, from, "alice"); !errors.Is(err, ErrAllocationRunPeriod) {
		t.Fatalf("period backwards = %v", err)
	}
	if _, err := NewService(&fakeRepo{err: ErrCostCenterNotFound}, okAudit{}).AllocateToCostUnits(ctx, 1, from, to, "alice"); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v", err)
	}

	// No key in force means nothing to distribute.
	stale := []CostUnitKey{{ID: 1, TargetUnitID: 2, ShareBP: 10000, ValidFrom: day(2026, 1, 1), ValidTo: ptr(day(2026, 3, 1))}}
	if _, err := NewService(&fakeRepo{unitKeys: stale, costs: 5000}, okAudit{}).AllocateToCostUnits(ctx, 1, from, to, "alice"); !errors.Is(err, ErrAllocationRunNoTargets) {
		t.Fatalf("no eligible key = %v", err)
	}

	// A period with no costs is refused by the split, not by a second check in the service.
	noCosts := &fakeRepo{unitKeys: keys, costs: 0}
	if _, err := NewService(noCosts, okAudit{}).AllocateToCostUnits(ctx, 1, from, to, "alice"); !errors.Is(err, ErrNothingToAllocate) {
		t.Fatalf("no costs = %v", err)
	}

	boom := errors.New("boom")
	if _, err := NewService(&fakeRepo{unitKeys: keys, costs: 5000, err: boom}, okAudit{}).AllocateToCostUnits(ctx, 1, from, to, "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v", err)
	}
	if _, err := NewService(&fakeRepo{unitKeys: keys, costs: 5000}, failingAudit{err: boom}).AllocateToCostUnits(ctx, 1, from, to, "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v", err)
	}
}

func ptr(t time.Time) *time.Time { return &t }
