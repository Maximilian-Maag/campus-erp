package klr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// TestKostentraegerRun is the acceptance test for the Kostenträgerrechnung: a cost centre's
// period costs are attributed to its cost units, the centre's balance is left where it was, and
// the same period cannot be attributed twice.
func TestKostentraegerRun(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	center, err := svc.CreateCostCenter(ctx, "K-"+mark, "Verwaltung", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	unitA, err := svc.CreateCostUnit(ctx, "TA-"+mark, "Studiengang A", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}
	unitB, err := svc.CreateCostUnit(ctx, "TB-"+mark, "Studiengang B", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}

	// 1000,00 in the period, plus one cost outside it that must not be touched.
	if _, err := svc.Post(ctx, center.ID, nil, nil, klr.Cost, 100000, klrDay(2026, 4, 10), "RE-1-"+mark, "alice"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if _, err := svc.Post(ctx, center.ID, nil, nil, klr.Cost, 7777, klrDay(2026, 5, 10), "RE-2-"+mark, "alice"); err != nil {
		t.Fatalf("Post: %v", err)
	}

	// The keys: three quarters to A, one quarter to B.
	if _, err := svc.DefineCostUnitKey(ctx, center.ID, unitA.ID, 7500, klrDay(2026, 1, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineCostUnitKey: %v", err)
	}
	if _, err := svc.DefineCostUnitKey(ctx, center.ID, unitB.ID, 2500, klrDay(2026, 1, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineCostUnitKey: %v", err)
	}

	keys, err := svc.CostUnitKeys(ctx, center.ID)
	if err != nil || len(keys) != 2 {
		t.Fatalf("CostUnitKeys = %+v, %v", keys, err)
	}

	before, err := svc.Report(ctx, center.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	run, err := svc.AllocateToCostUnits(ctx, center.ID, klrDay(2026, 4, 1), klrDay(2026, 4, 30), "alice")
	if err != nil {
		t.Fatalf("AllocateToCostUnits: %v", err)
	}
	if run.AllocatedCents != 100000 || run.Receivers != 2 {
		t.Fatalf("run = %+v", run)
	}

	// The source centre's balance is exactly where it was: the tagged costs and the relief
	// cancel, so attributing a centre's costs does not distort the centre.
	after, err := svc.Report(ctx, center.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if after.BalanceCents() != before.BalanceCents() {
		t.Fatalf("centre balance moved: %d -> %d", before.BalanceCents(), after.BalanceCents())
	}

	// The units now carry their shares.
	figures, err := svc.CostUnitFigures(ctx)
	if err != nil {
		t.Fatalf("CostUnitFigures: %v", err)
	}
	got := map[int64]int64{}
	for _, line := range figures {
		got[line.CostUnit.ID] = line.CostsCents
	}
	if got[unitA.ID] != 75000 || got[unitB.ID] != 25000 {
		t.Fatalf("unit costs = A %d, B %d; want 75000 and 25000", got[unitA.ID], got[unitB.ID])
	}

	// The same period cannot be attributed twice.
	if _, err := svc.AllocateToCostUnits(ctx, center.ID, klrDay(2026, 4, 1), klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrAlreadyAllocated) {
		t.Fatalf("second run = %v, want ErrAlreadyAllocated", err)
	}
	// Neither can a period overlapping it.
	if _, err := svc.AllocateToCostUnits(ctx, center.ID, klrDay(2026, 4, 15), klrDay(2026, 5, 15), "alice"); !errors.Is(err, klr.ErrAlreadyAllocated) {
		t.Fatalf("overlapping run = %v, want ErrAlreadyAllocated", err)
	}

	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestCostUnitKeyRefusals: the key is validated against the store, not just on its own.
func TestCostUnitKeyRefusals(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	center, err := svc.CreateCostCenter(ctx, "K-"+mark, "Verwaltung", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	unit, err := svc.CreateCostUnit(ctx, "T-"+mark, "Studiengang A", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}

	if _, err := svc.DefineCostUnitKey(ctx, center.ID, 9223372036854775807, 10000, klrDay(2026, 1, 1), nil, "alice"); !errors.Is(err, klr.ErrCostUnitNotFound) {
		t.Fatalf("unknown unit = %v, want ErrCostUnitNotFound", err)
	}
	if _, err := svc.DefineCostUnitKey(ctx, 9223372036854775807, unit.ID, 10000, klrDay(2026, 1, 1), nil, "alice"); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v, want ErrCostCenterNotFound", err)
	}
	// A key whose period has already closed does not distribute a later period.
	if _, err := svc.DefineCostUnitKey(ctx, center.ID, unit.ID, 10000, klrDay(2026, 1, 1), ptrTime(klrDay(2026, 3, 1)), "alice"); err != nil {
		t.Fatalf("DefineCostUnitKey: %v", err)
	}
	if _, err := svc.AllocateToCostUnits(ctx, center.ID, klrDay(2026, 4, 1), klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrAllocationRunNoTargets) {
		t.Fatalf("expired key = %v, want ErrAllocationRunNoTargets", err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestCreateCostUnitKeyTurnsAMissingUnitIntoANotFound: the store's foreign-key violation is mapped
// to the domain's not-found. The service checks both ends first, so this branch is only reached if
// that check and the insert disagree — a race — which is exactly why the mapping is there. It is
// exercised here at the store, where the violation can be asked for directly.
func TestCreateCostUnitKeyTurnsAMissingUnitIntoANotFound(t *testing.T) {
	ctx := context.Background()
	repo := klr.NewRepo(testPool(t))

	center, err := repo.CreateCostCenter(ctx, klr.CostCenter{Number: "K-S6-" + suffix(t), Name: "Serverbetrieb", CreatedBy: "alice"})
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	key, err := klr.NewCostUnitKey(center.ID, 9223372036854775807, 10000, klrDay(2026, 11, 1), nil)
	if err != nil {
		t.Fatalf("NewCostUnitKey: %v", err)
	}
	if _, err := repo.CreateCostUnitKey(ctx, key); !errors.Is(err, klr.ErrCostUnitNotFound) {
		t.Fatalf("got %v, want ErrCostUnitNotFound", err)
	}
}
