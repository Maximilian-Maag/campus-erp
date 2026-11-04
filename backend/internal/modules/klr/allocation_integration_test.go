package klr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

func klrDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// period is a month no other test in this package uses, so a run never collides with another
// test's.
func period(t *testing.T) (time.Time, time.Time) {
	t.Helper()
	from := klrDay(2026, 4, 1)
	return from, klrDay(2026, 4, 30)
}

// balancesOf sums what the given centres are up or down, which is what a distribution must not
// change: a run moves cost between centres, it neither creates nor destroys any. (The cost
// volume does rise by the allocated amount — the receivers' new records — while the source is
// relieved through a revenue, because records are append-only. The balance is the invariant.)
func balancesOf(t *testing.T, svc *klr.Service, centers ...klr.CostCenter) int64 {
	t.Helper()
	var total int64
	for _, c := range centers {
		rep, err := svc.Report(context.Background(), c.ID)
		if err != nil {
			t.Fatalf("Report(%d): %v", c.ID, err)
		}
		total += rep.BalanceCents()
	}
	return total
}

// TestAllocateDistributesCosts is the acceptance test: a cost centre's period costs reach its
// receivers in the shares its keys give, the source is relieved by the same amount, and the
// KLR's total cost is unchanged — the money moved, it was not created.
func TestAllocateDistributesCosts(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	from, to := period(t)

	source, err := svc.CreateCostCenter(ctx, "K-SRC-"+suffix(t), "Serverbetrieb", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter source: %v", err)
	}
	receiverA, err := svc.CreateCostCenter(ctx, "K-A-"+suffix(t), "Fachbereich A", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter A: %v", err)
	}
	receiverB, err := svc.CreateCostCenter(ctx, "K-B-"+suffix(t), "Fachbereich B", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter B: %v", err)
	}

	if _, err := svc.DefineAllocation(ctx, source.ID, receiverA.ID, 7500, from, &to, "alice"); err != nil {
		t.Fatalf("DefineAllocation A: %v", err)
	}
	if _, err := svc.DefineAllocation(ctx, source.ID, receiverB.ID, 2500, from, &to, "alice"); err != nil {
		t.Fatalf("DefineAllocation B: %v", err)
	}
	keys, err := svc.Allocations(ctx, source.ID)
	if err != nil {
		t.Fatalf("Allocations: %v", err)
	}
	if len(keys) != 2 || keys[0].TargetCenterID != receiverA.ID || keys[1].ShareBP != 2500 {
		t.Fatalf("keys = %+v", keys)
	}

	// Costs on the source inside the period, and one on another day that must not be part of
	// this distribution.
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 100000, from, "Strom", "alice"); err != nil {
		t.Fatalf("post cost: %v", err)
	}
	// A cost from the month before is not part of this run.
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 99999, klrDay(2026, 3, 15), "Vormonat", "alice"); err != nil {
		t.Fatalf("post previous cost: %v", err)
	}
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Revenue, 5000, from, "Erstattung", "alice"); err != nil {
		t.Fatalf("post revenue: %v", err)
	}

	before := balancesOf(t, svc, source, receiverA, receiverB)

	run, err := svc.Allocate(ctx, source.ID, from, to, "carol")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if run.AllocatedCents != 100000 || run.Receivers != 2 || run.RunBy != "carol" || run.ID == 0 {
		t.Fatalf("run = %+v", run)
	}
	if !run.PeriodFrom.Equal(from) || !run.PeriodTo.Equal(to) {
		t.Fatalf("run period = %v..%v", run.PeriodFrom, run.PeriodTo)
	}

	// The source is relieved by exactly what it handed out: its costs remain, its revenue grew,
	// and the two cancel.
	srcReport, err := svc.Report(ctx, source.ID)
	if err != nil {
		t.Fatalf("Report source: %v", err)
	}
	if srcReport.CostsCents != 199999 || srcReport.RevenuesCents != 105000 || srcReport.BalanceCents() != -94999 {
		t.Fatalf("source report = %+v", srcReport)
	}

	// The receivers carry their shares.
	reportA, err := svc.Report(ctx, receiverA.ID)
	if err != nil {
		t.Fatalf("Report A: %v", err)
	}
	if reportA.CostsCents != 75000 {
		t.Fatalf("receiver A costs = %d, want 75000", reportA.CostsCents)
	}
	reportB, err := svc.Report(ctx, receiverB.ID)
	if err != nil {
		t.Fatalf("Report B: %v", err)
	}
	if reportB.CostsCents != 25000 {
		t.Fatalf("receiver B costs = %d, want 25000", reportB.CostsCents)
	}

	// The invariant: redistributing moves cost, it does not change what the KLR is up or down.
	if after := balancesOf(t, svc, source, receiverA, receiverB); after != before {
		t.Fatalf("the balance of the three centres changed by the distribution: %d -> %d", before, after)
	}
}

// TestAllocateRefusesASecondRun: the same period cannot be distributed twice — the costs must
// not be booked again — and the refusal leaves the books exactly as they were.
func TestAllocateRefusesASecondRun(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	from, to := klrDay(2026, 5, 1), klrDay(2026, 5, 31)

	source, err := svc.CreateCostCenter(ctx, "K-S2-"+suffix(t), "Serverbetrieb", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	receiver, err := svc.CreateCostCenter(ctx, "K-R2-"+suffix(t), "Fachbereich", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 10000, from, &to, "alice"); err != nil {
		t.Fatalf("DefineAllocation: %v", err)
	}
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 40000, from, "Miete", "alice"); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, from, to, "carol"); err != nil {
		t.Fatalf("first Allocate: %v", err)
	}

	before := balancesOf(t, svc, source, receiver)
	if _, err := svc.Allocate(ctx, source.ID, from, to, "carol"); !errors.Is(err, klr.ErrAlreadyAllocated) {
		t.Fatalf("second Allocate = %v, want ErrAlreadyAllocated", err)
	}
	if after := balancesOf(t, svc, source, receiver); after != before {
		t.Fatalf("a refused run changed the books: %d -> %d", before, after)
	}

	// A window that overlaps the first one is refused as well: the same cost must not be handed
	// out twice, and the guard is the period's overlap, not the exact pair of dates.
	if _, err := svc.Allocate(ctx, source.ID, from, klrDay(2026, 5, 15), "carol"); !errors.Is(err, klr.ErrAlreadyAllocated) {
		t.Fatalf("overlapping run = %v, want ErrAlreadyAllocated", err)
	}
	// A window that does not overlap is free to run — but only once it has keys and costs of
	// its own.
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 6, 1), klrDay(2026, 6, 30), "carol"); !errors.Is(err, klr.ErrAllocationRunNoTargets) {
		t.Fatalf("June = %v, want ErrAllocationRunNoTargets (the key ended in May)", err)
	}
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 10000, klrDay(2026, 6, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineAllocation June: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 6, 1), klrDay(2026, 6, 30), "carol"); !errors.Is(err, klr.ErrNothingToAllocate) {
		t.Fatalf("June without costs = %v, want ErrNothingToAllocate", err)
	}
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 20000, klrDay(2026, 6, 10), "Miete Juni", "alice"); err != nil {
		t.Fatalf("post June: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 6, 1), klrDay(2026, 6, 30), "carol"); err != nil {
		t.Fatalf("June run: %v", err)
	}
}

// TestAllocateRefusals: shares that do not add up, nothing to distribute, and no key in force.
func TestAllocateRefusals(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	source, err := svc.CreateCostCenter(ctx, "K-S3-"+suffix(t), "Serverbetrieb", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	receiver, err := svc.CreateCostCenter(ctx, "K-R3-"+suffix(t), "Fachbereich", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}

	// No key at all.
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 7, 1), klrDay(2026, 7, 31), "carol"); !errors.Is(err, klr.ErrAllocationRunNoTargets) {
		t.Fatalf("no keys = %v, want ErrAllocationRunNoTargets", err)
	}
	// An unknown cost centre.
	if _, err := svc.Allocate(ctx, 9223372036854775807, klrDay(2026, 7, 1), klrDay(2026, 7, 31), "carol"); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v, want ErrCostCenterNotFound", err)
	}
	// A period that runs backwards, and a blank actor.
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 7, 31), klrDay(2026, 7, 1), "carol"); !errors.Is(err, klr.ErrAllocationRunPeriod) {
		t.Fatalf("backwards period = %v, want ErrAllocationRunPeriod", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 7, 1), klrDay(2026, 7, 31), "  "); !errors.Is(err, klr.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}

	// Shares that add up to less than everything.
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 6000, klrDay(2026, 8, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineAllocation: %v", err)
	}
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 1000, klrDay(2026, 8, 15), "RE", "alice"); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 8, 1), klrDay(2026, 8, 31), "carol"); !errors.Is(err, klr.ErrSharesIncomplete) {
		t.Fatalf("incomplete shares = %v, want ErrSharesIncomplete", err)
	}

	// A period with no costs in it.
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 4000, klrDay(2026, 8, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineAllocation: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 9, 1), klrDay(2026, 9, 30), "carol"); !errors.Is(err, klr.ErrNothingToAllocate) {
		t.Fatalf("empty period = %v, want ErrNothingToAllocate", err)
	}

	// A key that expired before the period is not in force.
	expired := klrDay(2026, 3, 31)
	if _, err := svc.DefineAllocation(ctx, receiver.ID, source.ID, 10000, klrDay(2026, 1, 1), &expired, "alice"); err != nil {
		t.Fatalf("DefineAllocation expired: %v", err)
	}
	if _, err := svc.Allocate(ctx, receiver.ID, klrDay(2026, 3, 1), klrDay(2026, 3, 31), "carol"); !errors.Is(err, klr.ErrNothingToAllocate) {
		t.Fatalf("expired key with no costs = %v", err)
	}

	// Defining a key against an unknown centre is a clear not-found, not a foreign-key failure.
	if _, err := svc.DefineAllocation(ctx, source.ID, 9223372036854775807, 1000, klrDay(2026, 1, 1), nil, "alice"); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("unknown target = %v, want ErrCostCenterNotFound", err)
	}
	if _, err := svc.DefineAllocation(ctx, 9223372036854775807, receiver.ID, 1000, klrDay(2026, 1, 1), nil, "alice"); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("unknown source = %v, want ErrCostCenterNotFound", err)
	}
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 1000, klrDay(2026, 1, 1), nil, "  "); !errors.Is(err, klr.ErrActorRequired) {
		t.Fatalf("blank actor on define = %v, want ErrActorRequired", err)
	}
}

// TestAllocateIsOnTheAuditChain: a run is a domain event like any other, and the chain still
// verifies afterwards.
func TestAllocateIsOnTheAuditChain(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	svc := klr.NewService(klr.NewRepo(pool), store)

	source, err := svc.CreateCostCenter(ctx, "K-S4-"+suffix(t), "Serverbetrieb", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	receiver, err := svc.CreateCostCenter(ctx, "K-R4-"+suffix(t), "Fachbereich", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	if _, err := svc.DefineAllocation(ctx, source.ID, receiver.ID, 10000, klrDay(2026, 10, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineAllocation: %v", err)
	}
	if _, err := svc.Post(ctx, source.ID, nil, nil, klr.Cost, 12345, klrDay(2026, 10, 15), "RE", "alice"); err != nil {
		t.Fatalf("post: %v", err)
	}
	if _, err := svc.Allocate(ctx, source.ID, klrDay(2026, 10, 1), klrDay(2026, 10, 31), "carol"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestCreateAllocationTurnsAMissingCentreIntoANotFound: the store's foreign-key violation is
// mapped to the domain's not-found. The service checks both ends first, so this branch is only
// reached if that check and the insert disagree — a race — which is exactly why the mapping is
// there. It is exercised here at the store, where the violation can be asked for directly.
func TestCreateAllocationTurnsAMissingCentreIntoANotFound(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo := klr.NewRepo(pool)

	center, err := repo.CreateCostCenter(ctx, klr.CostCenter{Number: "K-S5-" + suffix(t), Name: "Serverbetrieb", CreatedBy: "alice"})
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	key, err := klr.NewAllocation(center.ID, 9223372036854775807, 10000, klrDay(2026, 11, 1), nil)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	if _, err := repo.CreateAllocation(ctx, key); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("got %v, want ErrCostCenterNotFound", err)
	}
}
