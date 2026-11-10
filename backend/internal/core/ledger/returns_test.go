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

func fixedReturns() ledger.ReturnExport {
	return ledger.ReturnExport{
		From:        day(1),
		To:          day(31),
		CashAccount: "1200",
		Postings: []ledger.ReturnPosting{
			{
				Reference: "RT-000002", ValueDate: day(2), BookingDate: day(3),
				Account: "1300", Direction: ledger.Debit, AmountCents: 12500,
				PostingKey: "009", OrderReference: "ANORD-2026-9", Purpose: "Rückläufer Lastschrift",
			},
			{
				Reference: "RT-000001", ValueDate: day(1),
				Account: "1300", Direction: ledger.Credit, AmountCents: 4999,
				PostingKey: "059", OrderReference: "ANORD-2026-4", Purpose: "Rückläufer Zahlung",
			},
		},
	}
}

// TestMarshalReturnsGolden pins the exact bytes of the return postings file.
// Regenerate with UPDATE_GOLDEN=1.
func TestMarshalReturnsGolden(t *testing.T) {
	got, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_returns.csv"), []byte(got))
}

func TestMarshalReturnsDeterministic(t *testing.T) {
	a, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Marshal is not deterministic")
	}

	// Collecting the same returns in another order must produce the same file: the
	// sort key is (value date, reference).
	e := fixedReturns()
	e.Postings[0], e.Postings[1] = e.Postings[1], e.Postings[0]
	c, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != c {
		t.Fatalf("order changed the output:\n%s\n---\n%s", a, c)
	}
}

// Same value date: the reference decides, so the order is total rather than merely stable.
func TestMarshalReturnsSortsByReferenceOnTheSameDay(t *testing.T) {
	e := fixedReturns()
	for i := range e.Postings {
		e.Postings[i].ValueDate = day(1)
	}
	got, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(got, "RT-000001") > strings.Index(got, "RT-000002") {
		t.Fatalf("references not ordered on the same value date:\n%s", got)
	}
}

func TestMarshalReturnsDefaultsCurrency(t *testing.T) {
	got, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ",EUR,") {
		t.Fatalf("expected the default currency:\n%s", got)
	}

	e := fixedReturns()
	e.Currency = "chf"
	got, err = e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, ",CHF,") {
		t.Fatalf("currency must be upper-cased:\n%s", got)
	}
}

func TestMarshalReturnsQuotesCSV(t *testing.T) {
	e := fixedReturns()
	e.Postings[0].Purpose = `Retour, "geplatzt"`
	got, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"Retour, ""geplatzt"""`) {
		t.Fatalf("purpose not quoted/escaped:\n%s", got)
	}
}

func TestMarshalReturnsTrimsFields(t *testing.T) {
	e := fixedReturns()
	e.CashAccount = "  1200  "
	e.Postings[0].Reference = "  RT-000002  "
	e.Postings[0].PostingKey = "  009  "
	got, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "# cash_account: 1200\n") || !strings.Contains(got, "RT-000002,2026-10-02") {
		t.Fatalf("fields must be trimmed:\n%s", got)
	}
	if !strings.Contains(got, ",009,") {
		t.Fatalf("posting key must be trimmed:\n%s", got)
	}
}

// A line the bank sent without a booking date is written with an empty one rather than
// with a zero date, so the file never claims a day the statement did not name.
func TestMarshalReturnsOmitsAnAbsentBookingDate(t *testing.T) {
	got, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "RT-000001,2026-10-01,,1300,1200,credit,4999,EUR,059,") {
		t.Fatalf("a missing booking date must be empty:\n%s", got)
	}
}

func TestValidateReturns(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ledger.ReturnExport)
		want   error
	}{
		{"empty cash account", func(e *ledger.ReturnExport) { e.CashAccount = "  " }, ledger.ErrCashAccountRequired},
		{"no postings", func(e *ledger.ReturnExport) { e.Postings = nil }, ledger.ErrNoReturnPostings},
		{"period inverted", func(e *ledger.ReturnExport) { e.From, e.To = day(31), day(1) }, ledger.ErrPeriodInvalid},
		{"bad currency", func(e *ledger.ReturnExport) { e.Currency = "EURO" }, ledger.ErrCurrencyInvalid},
		{"empty reference", func(e *ledger.ReturnExport) { e.Postings[0].Reference = " " }, ledger.ErrReferenceRequired},
		{"zero value date", func(e *ledger.ReturnExport) { e.Postings[0].ValueDate = time.Time{} }, ledger.ErrReturnDateRequired},
		{"empty account", func(e *ledger.ReturnExport) { e.Postings[0].Account = "  " }, ledger.ErrReturnAccountRequired},
		{"bad direction", func(e *ledger.ReturnExport) { e.Postings[0].Direction = "both" }, ledger.ErrDirectionInvalid},
		{"zero amount", func(e *ledger.ReturnExport) { e.Postings[0].AmountCents = 0 }, ledger.ErrAmountNotPositive},
		{"negative amount", func(e *ledger.ReturnExport) { e.Postings[0].AmountCents = -1 }, ledger.ErrAmountNotPositive},
		{"empty posting key", func(e *ledger.ReturnExport) { e.Postings[0].PostingKey = "  " }, ledger.ErrReturnPostingKeyNeeded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := fixedReturns()
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

func TestValidateReturnsAcceptsOpenPeriod(t *testing.T) {
	e := fixedReturns()
	e.From, e.To = time.Time{}, time.Time{}
	if err := e.Validate(); err != nil {
		t.Fatalf("an open period must be accepted: %v", err)
	}
}

func TestReturnsFormatVersionIsWritten(t *testing.T) {
	got, err := fixedReturns().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "# "+ledger.ReturnsFormatVersion+"\n") {
		t.Fatalf("the version must be the first line:\n%s", got)
	}
	if !strings.Contains(got, "# period: 2026-10-01..2026-10-31\n") {
		t.Fatalf("the period must be stated:\n%s", got)
	}
}
