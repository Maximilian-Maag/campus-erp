package klr

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// TestWritesFailWhenTheAuditFails: a write that cannot be recorded must not be reported as
// done, so the service hands the audit's error back rather than swallowing it. The record or
// the run is stored either way — the trail is what makes it explainable, and losing an event
// silently is the one outcome nobody could later reconstruct.
func TestWritesFailWhenTheAuditFails(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("audit down")
	from, to := day(2026, 4, 1), day(2026, 4, 30)

	key, err := NewAllocation(1, 2, 10000, from, &to)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	center := CostCenter{ID: 1, Number: "4711", Name: "Bibliothek"}

	cases := []struct {
		name string
		repo *fakeRepo
		call func(*Service) error
	}{
		{
			name: "posting a record",
			repo: &fakeRepo{center: center, record: Record{ID: 1, Kind: Cost, AmountCents: 100, BusinessDate: from}},
			call: func(s *Service) error {
				_, err := s.Post(ctx, 1, nil, nil, Cost, 100, from, "RE-1", "alice")
				return err
			},
		},
		{
			name: "defining a key",
			repo: &fakeRepo{center: center},
			call: func(s *Service) error {
				_, err := s.DefineAllocation(ctx, 1, 2, 10000, from, &to, "alice")
				return err
			},
		},
		{
			name: "running a distribution",
			repo: &fakeRepo{center: center, allocations: []Allocation{key}, costs: 50000},
			call: func(s *Service) error {
				_, err := s.Allocate(ctx, 1, from, to, "alice")
				return err
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewService(c.repo, failingAudit{err: boom})
			if err := c.call(svc); !errors.Is(err, boom) {
				t.Fatalf("got %v, want the audit error", err)
			}
		})
	}
}

// TestAllocateRecordsTheRun: with a working audit the run goes through and reports what it
// distributed.
func TestAllocateRecordsTheRun(t *testing.T) {
	ctx := context.Background()
	from, to := day(2026, 4, 1), day(2026, 4, 30)
	key, err := NewAllocation(1, 2, 10000, from, &to)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	repo := &fakeRepo{center: CostCenter{ID: 1, Number: "4711"}, allocations: []Allocation{key}, costs: 50000}
	svc := NewService(repo, okAudit{})

	run, err := svc.Allocate(ctx, 1, from, to, "alice")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if run.AllocatedCents != 50000 || run.Receivers != 1 || run.RunBy != "alice" || run.ID == 0 {
		t.Fatalf("run = %+v", run)
	}
	if !run.PeriodFrom.Equal(from) || !run.PeriodTo.Equal(to) {
		t.Fatalf("period = %v..%v", run.PeriodFrom, run.PeriodTo)
	}
	if run.SourceCenterID != 1 {
		t.Fatalf("source = %d", run.SourceCenterID)
	}
}

// TestAllocateRollsBackWhenTheRepoRefuses: a run the store rejects (an overlapping period) is
// handed back as it is, not as a run that happened.
func TestAllocateRollsBackWhenTheRepoRefuses(t *testing.T) {
	ctx := context.Background()
	from, to := day(2026, 4, 1), day(2026, 4, 30)
	key, err := NewAllocation(1, 2, 10000, from, &to)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	repo := &fakeRepo{
		center: CostCenter{ID: 1, Number: "4711"}, allocations: []Allocation{key},
		costs: 50000, err: ErrAlreadyAllocated,
	}
	svc := NewService(repo, okAudit{})

	if _, err := svc.Allocate(ctx, 1, from, to, "alice"); !errors.Is(err, ErrAlreadyAllocated) {
		t.Fatalf("got %v, want ErrAlreadyAllocated", err)
	}
}

// TestDefineAllocationReportsTheStoredKey: the key that comes back is the one that was stored,
// with the values that were checked.
func TestDefineAllocationReportsTheStoredKey(t *testing.T) {
	ctx := context.Background()
	from, to := day(2026, 4, 1), day(2026, 4, 30)
	repo := &fakeRepo{center: CostCenter{ID: 1, Number: "4711"}, allocation: Allocation{ID: 7}}
	svc := NewService(repo, okAudit{})

	key, err := svc.DefineAllocation(ctx, 1, 2, 2500, from, &to, "alice")
	if err != nil {
		t.Fatalf("DefineAllocation: %v", err)
	}
	if key.ID != 7 || key.SourceCenterID != 1 || key.TargetCenterID != 2 || key.ShareBP != 2500 {
		t.Fatalf("key = %+v", key)
	}
	if key.CreatedBy != "alice" || !key.ValidFrom.Equal(from) || key.ValidTo == nil || !key.ValidTo.Equal(to) {
		t.Fatalf("key = %+v", key)
	}
}

// TestAllocateRefusesAPeriodWithoutKeys: a centre whose keys have all expired has nothing to
// distribute over, and saying so is better than distributing nowhere.
func TestAllocateRefusesAPeriodWithoutKeys(t *testing.T) {
	ctx := context.Background()
	from, to := day(2026, 4, 1), day(2026, 4, 30)
	expired := day(2026, 3, 31)
	old, err := NewAllocation(1, 2, 10000, day(2026, 1, 1), &expired)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	repo := &fakeRepo{center: CostCenter{ID: 1}, allocations: []Allocation{old}, costs: 50000}
	svc := NewService(repo, okAudit{})

	if _, err := svc.Allocate(ctx, 1, from, to, "alice"); !errors.Is(err, ErrAllocationRunNoTargets) {
		t.Fatalf("got %v, want ErrAllocationRunNoTargets", err)
	}
}

// okAudit accepts every event.
type okAudit struct{}

func (okAudit) Append(_ context.Context, e audit.Event) (audit.Event, error) { return e, nil }
