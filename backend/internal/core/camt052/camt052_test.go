package camt052

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/bankstmt"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

func TestParseGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "golden_camt052.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := Parse(string(b))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("reports = %d, want 1", len(stmts))
	}
	st := stmts[0]
	if st.Reference != "RPT-2026-0001" || st.Account != "DE02120300000000202051" ||
		st.Currency != "EUR" || st.Number != 7 {
		t.Fatalf("report = %+v", st)
	}
	if st.OpeningBalanceCents != 1000000 || st.ClosingBalanceCents != 1120001 {
		t.Fatalf("balances = %d / %d", st.OpeningBalanceCents, st.ClosingBalanceCents)
	}
	if len(st.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(st.Lines))
	}
	c, d := st.Lines[0], st.Lines[1]
	if !c.Credit || c.AmountCents != 125000 || c.EndToEndRef != "AO-2026-0001" ||
		c.ClientRef != "TX-1" || c.CounterpartyName != "Stadtwerke Musterstadt" ||
		c.CounterpartyIBAN != "DE89370400440532013000" || c.Purpose != "Stromabschlag Oktober" ||
		!c.BookingDate.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("credit line = %+v", c)
	}
	if d.Credit || d.AmountCents != 4999 || d.CounterpartyName != "Buchhandlung Am Markt" {
		t.Fatalf("debit line = %+v", d)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse("<not-xml"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid = %v, want ErrInvalid", err)
	}
	if _, err := Parse("<Document><Other/></Document>"); !errors.Is(err, ErrNoReport) {
		t.Fatalf("no report = %v, want ErrNoReport", err)
	}
}

// TestParseWithoutBalances documents that a report without Bal yields zero
// balances (an intraday report need not carry them).
func TestParseWithoutBalances(t *testing.T) {
	const doc = `<Document><BkToCstmrAcctRpt><Rpt><Id>R1</Id>` +
		`<Acct><Id><IBAN>DE02120300000000202051</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Ntry><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>` +
		`<BookgDt><Dt>2026-10-28</Dt></BookgDt><ValDt><Dt>2026-10-28</Dt></ValDt></Ntry>` +
		`</Rpt></BkToCstmrAcctRpt></Document>`
	stmts, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	st := stmts[0]
	if st.OpeningBalanceCents != 0 || st.ClosingBalanceCents != 0 {
		t.Fatalf("balances must default to zero: %+v", st)
	}
	if len(st.Lines) != 1 || st.Lines[0].AmountCents != 1000 || !st.Lines[0].Credit {
		t.Fatalf("line = %+v", st.Lines[0])
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1250.00", 125000},
		{"49.99", 4999},
		{"0", 0},
		{"1,50", 150},
		{"-7", -700},
		{"abc", 0},
		{"", 0},
		{"1.2345", 123},
	}
	for _, c := range cases {
		if got := parseAmount(c.in); got != c.want {
			t.Errorf("parseAmount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	if got := parseDate("2026-10-28"); !got.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parseDate = %v", got)
	}
	if got := parseDate("2026-10-28T18:00:00"); !got.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parseDate(datetime) = %v", got)
	}
	if got := parseDate("nonsense"); !got.IsZero() {
		t.Errorf("parseDate(nonsense) = %v, want zero", got)
	}
}

// TestParseMatchesBankstmtShape is a compile-time guard that Parse feeds the
// shared shape.
func TestParseMatchesBankstmtShape(t *testing.T) {
	var _ []bankstmt.Statement = mustParse(t)
}

func mustParse(t *testing.T) []bankstmt.Statement {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden_camt052.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

// TestGoldenValidatesAgainstVendorXSD validates the golden camt.052.001.08 file against the
// vendor schema when ERP_XSD_CAMT052 points at it. The schema is proprietary and is not
// committed, so the check skips when the variable is unset (CI stays green); set it
// to the extracted XSD to actually validate.
func TestGoldenValidatesAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_CAMT052", filepath.Join("testdata", "golden_camt052.xml"))
}
