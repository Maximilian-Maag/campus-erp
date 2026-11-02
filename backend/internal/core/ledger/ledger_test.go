package ledger_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/ledger"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

func fixedExport() ledger.Export {
	return ledger.Export{
		From:        day(1),
		To:          day(31),
		CashAccount: "1200",
		Postings: []ledger.Posting{
			{Reference: "BK-2026-0002", BookingDate: day(2), Account: "4400", Direction: ledger.Debit, AmountCents: 4999, Purpose: "Fachliteratur"},
			{Reference: "BK-2026-0001", BookingDate: day(1), Account: "8400", Direction: ledger.Credit, AmountCents: 125000, Purpose: "Stromabschlag Oktober"},
		},
	}
}

// TestMarshalGolden pins the exact bytes. Regenerate with UPDATE_GOLDEN=1.
func TestMarshalGolden(t *testing.T) {
	got, err := fixedExport().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_postings.csv"), []byte(got))
}

func TestMarshalDeterministic(t *testing.T) {
	a, err := fixedExport().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fixedExport().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Marshal is not deterministic")
	}

	// Collecting the same bookings in another order must produce the same file: the
	// sort key is (booking date, reference).
	e := fixedExport()
	e.Postings[0], e.Postings[1] = e.Postings[1], e.Postings[0]
	c, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != c {
		t.Fatalf("order changed the output:\n%s\n---\n%s", a, c)
	}
}

func TestMarshalDefaultsCurrency(t *testing.T) {
	got, err := fixedExport().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ",EUR,") {
		t.Fatalf("expected the default currency:\n%s", got)
	}

	e := fixedExport()
	e.Currency = "chf"
	got, err = e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ",CHF,") {
		t.Fatalf("currency must be upper-cased:\n%s", got)
	}
}

func TestMarshalQuotesCSV(t *testing.T) {
	e := fixedExport()
	e.Postings[0].Purpose = `Miete, "Nebenkosten"`
	got, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"Miete, ""Nebenkosten"""`) {
		t.Fatalf("purpose not quoted/escaped:\n%s", got)
	}
}

func TestMarshalTrimsFields(t *testing.T) {
	e := fixedExport()
	e.CashAccount = "  1200  "
	e.Postings[0].Reference = "  BK-2026-0002  "
	got, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "# cash_account: 1200\n") || !strings.Contains(got, "BK-2026-0002,2026-10-02") {
		t.Fatalf("fields must be trimmed:\n%s", got)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ledger.Export)
		want   error
	}{
		{"empty cash account", func(e *ledger.Export) { e.CashAccount = "  " }, ledger.ErrCashAccountRequired},
		{"no postings", func(e *ledger.Export) { e.Postings = nil }, ledger.ErrNoPostings},
		{"period inverted", func(e *ledger.Export) { e.From, e.To = day(31), day(1) }, ledger.ErrPeriodInvalid},
		{"bad currency", func(e *ledger.Export) { e.Currency = "EURO" }, ledger.ErrCurrencyInvalid},
		{"empty reference", func(e *ledger.Export) { e.Postings[0].Reference = " " }, ledger.ErrReferenceRequired},
		{"zero booking date", func(e *ledger.Export) { e.Postings[0].BookingDate = time.Time{} }, ledger.ErrBookingDateRequired},
		{"empty account", func(e *ledger.Export) { e.Postings[0].Account = "  " }, ledger.ErrAccountRequired},
		{"bad direction", func(e *ledger.Export) { e.Postings[0].Direction = "both" }, ledger.ErrDirectionInvalid},
		{"zero amount", func(e *ledger.Export) { e.Postings[0].AmountCents = 0 }, ledger.ErrAmountNotPositive},
		{"negative amount", func(e *ledger.Export) { e.Postings[0].AmountCents = -1 }, ledger.ErrAmountNotPositive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := fixedExport()
			c.mutate(&e)
			if _, err := e.Marshal(); !errors.Is(err, c.want) {
				t.Fatalf("Marshal error = %v, want %v", err, c.want)
			}
			if err := e.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("Validate error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestValidateAcceptsOpenPeriod(t *testing.T) {
	e := fixedExport()
	e.From, e.To = time.Time{}, time.Time{}
	if err := e.Validate(); err != nil {
		t.Fatalf("an open period must be accepted: %v", err)
	}
}

func TestDirectionValid(t *testing.T) {
	for _, d := range []ledger.Direction{ledger.Debit, ledger.Credit} {
		if !d.Valid() {
			t.Errorf("%q must be valid", d)
		}
	}
	for _, d := range []ledger.Direction{"", "Debit", "both"} {
		if d.Valid() {
			t.Errorf("%q must be invalid", d)
		}
	}
}
