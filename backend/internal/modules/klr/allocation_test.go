package klr

import (
	"errors"
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestNewAllocation: a key names a source and a different target, a share in basis points and a
// validity that does not run backwards.
func TestNewAllocation(t *testing.T) {
	from, to := day(2026, 1, 1), day(2026, 12, 31)

	got, err := NewAllocation(1, 2, 2500, from, &to)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	if got.SourceCenterID != 1 || got.TargetCenterID != 2 || got.ShareBP != 2500 {
		t.Fatalf("key = %+v", got)
	}
	if got.ValidTo == nil || !got.ValidTo.Equal(to) {
		t.Fatalf("valid_to = %v", got.ValidTo)
	}
	if open, err := NewAllocation(1, 2, 10000, from, nil); err != nil || open.ValidTo != nil {
		t.Fatalf("open-ended key = %+v, %v", open, err)
	}

	cases := []struct {
		name               string
		source, target, bp int64
		validFrom, validTo time.Time
		end                *time.Time
		want               error
	}{
		{"no source", 0, 2, 1000, from, time.Time{}, nil, ErrCostCenterNotFound},
		{"no target", 1, 0, 1000, from, time.Time{}, nil, ErrCostCenterNotFound},
		{"distributes to itself", 1, 1, 1000, from, time.Time{}, nil, ErrAllocationSelf},
		{"share above 100 percent", 1, 2, 10001, from, time.Time{}, nil, ErrAllocationShare},
		{"share below zero", 1, 2, -1, from, time.Time{}, nil, ErrAllocationShare},
		{"period runs backwards", 1, 2, 1000, to, time.Time{}, &from, ErrAllocationPeriod},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewAllocation(c.source, c.target, c.bp, c.validFrom, c.end); !errors.Is(err, c.want) {
				t.Fatalf("NewAllocation = %v, want %v", err, c.want)
			}
		})
	}

	// A zero share is a valid key: nothing goes to that receiver.
	if _, err := NewAllocation(1, 2, 0, from, nil); err != nil {
		t.Fatalf("zero share: %v", err)
	}
}

// TestAllocationActive: the whole validity range counts, and an open-ended key never expires.
func TestAllocationActive(t *testing.T) {
	from, to := day(2026, 4, 1), day(2026, 6, 30)
	key, err := NewAllocation(1, 2, 5000, from, &to)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	open, err := NewAllocation(1, 2, 5000, from, nil)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}

	if key.Active(day(2026, 3, 31)) {
		t.Fatal("the key is active before it starts")
	}
	if !key.Active(from) || !key.Active(to) || !key.Active(day(2026, 5, 15)) {
		t.Fatal("the key is not active inside its range (the bounds are inclusive)")
	}
	if key.Active(day(2026, 7, 1)) {
		t.Fatal("the key is still active after it ends")
	}
	if open.Active(day(2026, 3, 31)) || !open.Active(day(2030, 1, 1)) {
		t.Fatal("an open-ended key did not behave as open-ended")
	}
}

func keys(t *testing.T, shares ...int64) []Allocation {
	t.Helper()
	out := make([]Allocation, 0, len(shares))
	for i, bp := range shares {
		k, err := NewAllocation(1, int64(10+i), bp, day(2026, 1, 1), nil)
		if err != nil {
			t.Fatalf("NewAllocation: %v", err)
		}
		out = append(out, k)
	}
	return out
}

// TestSplitShares: the shares always add up to the amount exactly — no cent is created or lost
// to rounding — and the keys are split in the order given.
func TestSplitShares(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		shares []int64
		want   []int64
	}{
		{"even split", 10000, []int64{5000, 5000}, []int64{5000, 5000}},
		{"quarters", 10000, []int64{2500, 2500, 2500, 2500}, []int64{2500, 2500, 2500, 2500}},
		{"the remainder goes to the last", 100, []int64{3333, 3333, 3334}, []int64{33, 33, 34}},
		{"one cent", 1, []int64{5000, 5000}, []int64{0, 1}},
		{"a share that rounds to nothing", 2, []int64{4999, 5001}, []int64{0, 2}},
		{"uneven thirds", 1000, []int64{3334, 3333, 3333}, []int64{333, 333, 334}},
		{"a zero-share receiver", 100, []int64{0, 10000}, []int64{0, 100}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SplitShares(c.amount, keys(t, c.shares...))
			if err != nil {
				t.Fatalf("SplitShares: %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("shares = %d, want %d", len(got), len(c.want))
			}
			var total int64
			for i, sh := range got {
				if sh.Cents != c.want[i] {
					t.Fatalf("share %d = %d, want %d", i, sh.Cents, c.want[i])
				}
				total += sh.Cents
			}
			// The invariant the whole engine rests on.
			if total != c.amount {
				t.Fatalf("shares total %d, want the amount %d", total, c.amount)
			}
		})
	}
}

// TestSplitSharesRefusals: shares that do not add up, no keys at all, and nothing to split.
func TestSplitSharesRefusals(t *testing.T) {
	if _, err := SplitShares(1000, nil); !errors.Is(err, ErrAllocationRunNoTargets) {
		t.Fatalf("no keys = %v, want ErrAllocationRunNoTargets", err)
	}
	if _, err := SplitShares(0, keys(t, 10000)); !errors.Is(err, ErrNothingToAllocate) {
		t.Fatalf("nothing to split = %v, want ErrNothingToAllocate", err)
	}
	if _, err := SplitShares(-5, keys(t, 10000)); !errors.Is(err, ErrNothingToAllocate) {
		t.Fatalf("negative amount = %v, want ErrNothingToAllocate", err)
	}
	for _, shares := range [][]int64{{9000}, {5000, 4000}, {10000, 1}} {
		if _, err := SplitShares(1000, keys(t, shares...)); !errors.Is(err, ErrSharesIncomplete) {
			t.Fatalf("shares %v = %v, want ErrSharesIncomplete", shares, err)
		}
	}
	// A share outside the range cannot come through the constructor (and the table refuses it),
	// so the split checks the sum itself rather than trusting its caller.
	over := Allocation{SourceCenterID: 1, TargetCenterID: 10, ShareBP: 11000}
	if _, err := SplitShares(1000, []Allocation{over}); !errors.Is(err, ErrSharesIncomplete) {
		t.Fatalf("over-full share = %v, want ErrSharesIncomplete", err)
	}
}

// TestPlanBooksReceiversAndRelief: one cost per receiving centre for its share, one revenue on
// the source for the total handed out, and nothing booked for a share that rounds to nothing.
func TestPlanBooksReceiversAndRelief(t *testing.T) {
	records, err := plan(1, 100, keys(t, 3333, 3333, 3334), day(2026, 4, 30), "carol")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("records = %d, want three receivers plus the relief", len(records))
	}
	for i, want := range []int64{33, 33, 34} {
		rec := records[i]
		if rec.Kind != Cost || rec.AmountCents != want || rec.CostCenterID != int64(10+i) {
			t.Fatalf("receiver %d = %+v, want a cost of %d on centre %d", i, rec, want, 10+i)
		}
		if rec.BookedBy != "carol" {
			t.Fatalf("receiver %d booked by %q", i, rec.BookedBy)
		}
		// The records belong to the period that was distributed, not to the day it ran.
		if !rec.BusinessDate.Equal(day(2026, 4, 30)) {
			t.Fatalf("receiver %d business date = %v", i, rec.BusinessDate)
		}
	}
	relief := records[3]
	if relief.Kind != Revenue || relief.AmountCents != 100 || relief.CostCenterID != 1 {
		t.Fatalf("relief = %+v, want a revenue of 100 on the source", relief)
	}

	// A receiver whose share rounds to nothing gets no record at all — a record's amount must
	// be positive — and the relief still equals the whole amount.
	records, err = plan(1, 1, keys(t, 4999, 5001), day(2026, 4, 30), "carol")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want one receiver and the relief", records)
	}
	if records[0].CostCenterID != 11 || records[0].AmountCents != 1 {
		t.Fatalf("receiver = %+v", records[0])
	}
	if records[1].Kind != Revenue || records[1].AmountCents != 1 {
		t.Fatalf("relief = %+v", records[1])
	}
}

// TestPlanRefusesIncompleteShares: the engine's arithmetic is only as good as the keys.
func TestPlanRefusesIncompleteShares(t *testing.T) {
	if _, err := plan(1, 100, keys(t, 5000, 4000), day(2026, 4, 30), "carol"); !errors.Is(err, ErrSharesIncomplete) {
		t.Fatalf("plan = %v, want ErrSharesIncomplete", err)
	}
}

// TestRunReference: the records a run wrote are named after it, so a distributed cost is
// traceable in the report.
func TestRunReference(t *testing.T) {
	if runReference(7) != "UML:7" {
		t.Fatalf("runReference = %q", runReference(7))
	}
}

// TestAllocationString: the key renders readably in messages and logs.
func TestAllocationString(t *testing.T) {
	key, err := NewAllocation(1, 2, 2500, day(2026, 1, 1), nil)
	if err != nil {
		t.Fatalf("NewAllocation: %v", err)
	}
	key.ID = 9
	if got := key.String(); got != "allocation 9: centre 1 -> centre 2 at 2500 bp" {
		t.Fatalf("String = %q", got)
	}
}
