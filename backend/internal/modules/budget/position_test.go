package budget

import (
	"errors"
	"testing"
)

func TestNewPosition(t *testing.T) {
	p, err := NewPosition(2026, " 05 ", " 1234 ", " Beschaffung ", 100000)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	if p.Year != 2026 || p.Chapter != "05" || p.Title != "1234" || p.Description != "Beschaffung" || p.AllocatedCents != 100000 {
		t.Fatalf("position = %+v", p)
	}

	cases := []struct {
		name           string
		year           int
		chapter, title string
		allocated      int64
		want           error
	}{
		{"bad year", 1800, "05", "1", 0, ErrYearRequired},
		{"no chapter", 2026, "  ", "1", 0, ErrChapterRequired},
		{"no title", 2026, "05", "", 0, ErrTitleRequired},
		{"negative allocated", 2026, "05", "1", -1, ErrAllocatedNegative},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewPosition(c.year, c.chapter, c.title, "", c.allocated); !errors.Is(err, c.want) {
				t.Fatalf("NewPosition = %v, want %v", err, c.want)
			}
		})
	}
}

func TestNewPosting(t *testing.T) {
	p, err := NewPosting(7, Commitment, 5000, " PO-1 ")
	if err != nil {
		t.Fatalf("NewPosting: %v", err)
	}
	if p.PositionID != 7 || p.Kind != Commitment || p.AmountCents != 5000 || p.Reference != "PO-1" {
		t.Fatalf("posting = %+v", p)
	}

	if _, err := NewPosting(0, Commitment, 1, ""); !errors.Is(err, ErrPositionNotFound) {
		t.Fatalf("no position: %v", err)
	}
	if _, err := NewPosting(1, "gift", 1, ""); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("bad kind: %v", err)
	}
	if _, err := NewPosting(1, Actual, 0, ""); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("zero amount: %v", err)
	}
}

func TestBalanceAndCheckAvailable(t *testing.T) {
	b := Balance{PositionID: 1, AllocatedCents: 100000, CommittedCents: 30000, ActualCents: 20000}
	if b.AvailableCents() != 50000 {
		t.Fatalf("available = %d", b.AvailableCents())
	}
	if err := CheckAvailable(b, Commitment, 50000); err != nil {
		t.Fatalf("exact fit should pass: %v", err)
	}
	if err := CheckAvailable(b, Actual, 50001); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("overspend = %v", err)
	}
	// A release never exceeds.
	if err := CheckAvailable(b, Release, 999999); err != nil {
		t.Fatalf("release = %v", err)
	}
}
