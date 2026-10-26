package klr

import (
	"errors"
	"testing"
)

func TestNewCostCenterAndUnit(t *testing.T) {
	c, err := NewCostCenter(" 4711 ", " Bibliothek ")
	if err != nil {
		t.Fatalf("NewCostCenter: %v", err)
	}
	if c.Number != "4711" || c.Name != "Bibliothek" {
		t.Fatalf("centre = %+v", c)
	}
	u, err := NewCostUnit(" 9000 ", " Lehre ")
	if err != nil {
		t.Fatalf("NewCostUnit: %v", err)
	}
	if u.Number != "9000" || u.Name != "Lehre" {
		t.Fatalf("unit = %+v", u)
	}

	for _, tc := range []struct {
		name string
		fn   func() error
		want error
	}{
		{"centre no number", func() error { _, err := NewCostCenter("  ", "n"); return err }, ErrNumberRequired},
		{"centre no name", func() error { _, err := NewCostCenter("1", "  "); return err }, ErrNameRequired},
		{"unit no number", func() error { _, err := NewCostUnit("  ", "n"); return err }, ErrNumberRequired},
		{"unit no name", func() error { _, err := NewCostUnit("1", ""); return err }, ErrNameRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewRecord(t *testing.T) {
	cu := int64(5)
	rec, err := NewRecord(7, &cu, Cost, 1000, " RE-1 ")
	if err != nil {
		t.Fatalf("NewRecord: %v", err)
	}
	if rec.CostCenterID != 7 || rec.CostUnitID == nil || *rec.CostUnitID != 5 || rec.Kind != Cost || rec.AmountCents != 1000 || rec.Reference != "RE-1" {
		t.Fatalf("record = %+v", rec)
	}
	// A record without a cost unit is fine.
	if _, err := NewRecord(7, nil, Revenue, 1, ""); err != nil {
		t.Fatalf("no unit: %v", err)
	}

	if _, err := NewRecord(0, nil, Cost, 1, ""); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("no centre: %v", err)
	}
	if _, err := NewRecord(1, nil, "gift", 1, ""); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("bad kind: %v", err)
	}
	if _, err := NewRecord(1, nil, Cost, 0, ""); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("zero: %v", err)
	}
}

func TestReportBalance(t *testing.T) {
	r := Report{CostCenterID: 1, CostsCents: 30000, RevenuesCents: 50000}
	if r.BalanceCents() != 20000 {
		t.Fatalf("balance = %d", r.BalanceCents())
	}
	if (Report{CostsCents: 50000, RevenuesCents: 30000}).BalanceCents() != -20000 {
		t.Fatal("negative balance")
	}
}
