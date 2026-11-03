package procurement

import (
	"errors"
	"testing"
)

func lines() []Line {
	return []Line{
		{LineNo: 1, Description: "Ware", QuantityMilli: 1000, UnitPriceCents: 10000, NetCents: 10000, VatCategory: "S", VatRateBP: 1900},
		{LineNo: 2, Description: "Versand", QuantityMilli: 2000, UnitPriceCents: 500, NetCents: 1000, VatCategory: "S", VatRateBP: 1900},
	}
}

func TestLineNetAndTotals(t *testing.T) {
	if LineNet(1000, 10000) != 10000 || LineNet(2000, 500) != 1000 || LineNet(500, 333) != 167 {
		t.Fatalf("LineNet: %d %d %d", LineNet(1000, 10000), LineNet(2000, 500), LineNet(500, 333))
	}
	net, vat, gross := ComputeTotals(lines())
	if net != 11000 || vat != 2090 || gross != 13090 {
		t.Fatalf("totals = %d %d %d", net, vat, gross)
	}
	// Mixed rates produce separate VAT groups.
	mixed := []Line{
		{NetCents: 10000, VatCategory: "S", VatRateBP: 1900},
		{NetCents: 10000, VatCategory: "S", VatRateBP: 700},
	}
	if _, vat, _ := ComputeTotals(mixed); vat != 2600 {
		t.Fatalf("mixed vat = %d, want 2600", vat)
	}
}

// TestParseStatus: the API's status filter comes from this, so anything outside the lifecycle
// has to be refused rather than treated as "every status".
func TestParseStatus(t *testing.T) {
	for _, known := range Statuses() {
		got, err := ParseStatus(string(known))
		if err != nil || got != known {
			t.Fatalf("ParseStatus(%q) = %q, %v", known, got, err)
		}
	}
	if got, err := ParseStatus("  approved  "); err != nil || got != StatusApproved {
		t.Fatalf("ParseStatus did not ignore surrounding space: %q, %v", got, err)
	}
	for _, bad := range []string{"", "nope", "DRAFT", "open"} {
		if _, err := ParseStatus(bad); !errors.Is(err, ErrUnknownStatus) {
			t.Fatalf("ParseStatus(%q) = %v, want ErrUnknownStatus", bad, err)
		}
	}
	if len(Statuses()) != 4 || Statuses()[0] != StatusDraft {
		t.Fatalf("Statuses = %v", Statuses())
	}
}

func TestNewOrder(t *testing.T) {
	o, err := NewOrder(" PO-1 ", " Lieferant ", 7, "EUR", lines())
	if err != nil {
		t.Fatalf("NewOrder: %v", err)
	}
	if o.Number != "PO-1" || o.Supplier != "Lieferant" || o.BudgetPositionID != 7 || o.Status != StatusDraft {
		t.Fatalf("order = %+v", o)
	}
	if o.GrossCents != 13090 {
		t.Fatalf("gross = %d", o.GrossCents)
	}
}

func TestNewOrderValidation(t *testing.T) {
	base := func() (string, string, int64, string, []Line) {
		return "PO-1", "L", 7, "EUR", lines()
	}
	cases := []struct {
		name string
		mut  func(*string, *string, *int64, *string, *[]Line)
		want error
	}{
		{"no number", func(n, s *string, b *int64, c *string, l *[]Line) { *n = " " }, ErrNumberRequired},
		{"no supplier", func(n, s *string, b *int64, c *string, l *[]Line) { *s = " " }, ErrSupplierRequired},
		{"no budget", func(n, s *string, b *int64, c *string, l *[]Line) { *b = 0 }, ErrBudgetPositionRequired},
		{"bad currency", func(n, s *string, b *int64, c *string, l *[]Line) { *c = "EU" }, ErrCurrency},
		{"no lines", func(n, s *string, b *int64, c *string, l *[]Line) { *l = nil }, ErrNoLines},
		{"no desc", func(n, s *string, b *int64, c *string, l *[]Line) { (*l)[0].Description = " " }, ErrLineDescription},
		{"zero qty", func(n, s *string, b *int64, c *string, l *[]Line) { (*l)[0].QuantityMilli = 0 }, ErrLineQuantity},
		{"neg price", func(n, s *string, b *int64, c *string, l *[]Line) { (*l)[0].UnitPriceCents = -1; (*l)[0].NetCents = 0 }, ErrLineAmount},
		{"no vat cat", func(n, s *string, b *int64, c *string, l *[]Line) { (*l)[0].VatCategory = "" }, ErrVATCategoryRequired},
		{"net mismatch", func(n, s *string, b *int64, c *string, l *[]Line) { (*l)[0].NetCents = 1 }, ErrLineNetMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, s, b, cur, l := base()
			c.mut(&n, &s, &b, &cur, &l)
			if _, err := NewOrder(n, s, b, cur, l); !errors.Is(err, c.want) {
				t.Fatalf("NewOrder = %v, want %v", err, c.want)
			}
		})
	}
}
