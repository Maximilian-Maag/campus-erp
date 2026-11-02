package camt054

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
	b, err := os.ReadFile(filepath.Join("testdata", "golden_camt054.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := Parse(string(b))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(stmts))
	}
	st := stmts[0]
	if st.Reference != "NTC-2026-0001" || st.Account != "DE02120300000000202051" || st.Currency != "EUR" {
		t.Fatalf("statement = %+v", st)
	}
	// A camt.054 has no balance; both must be zero.
	if st.OpeningBalanceCents != 0 || st.ClosingBalanceCents != 0 {
		t.Fatalf("a notification must carry no balance: %+v", st)
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
	if d.Credit || d.AmountCents != 4999 || d.CounterpartyName != "Buchhandlung Am Markt" ||
		d.CounterpartyIBAN != "GB29NWBK60161331926819" || d.Purpose != "Fachliteratur" {
		t.Fatalf("debit line = %+v", d)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse("<not-xml"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid = %v, want ErrInvalid", err)
	}
	if _, err := Parse("<Document><Other/></Document>"); !errors.Is(err, ErrNoNotification) {
		t.Fatalf("no notification = %v, want ErrNoNotification", err)
	}
}

// TestParseEntryWithoutDetails covers an entry that carries no TxDtls.
func TestParseEntryWithoutDetails(t *testing.T) {
	const doc = `<Document><BkToCstmrDbtCdtNtfctn><Ntfctn><Id>N1</Id>` +
		`<Acct><Id><IBAN>DE02120300000000202051</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Ntry><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>` +
		`<BookgDt><Dt>2026-10-28</Dt></BookgDt><ValDt><Dt>2026-10-28</Dt></ValDt></Ntry>` +
		`</Ntfctn></BkToCstmrDbtCdtNtfctn></Document>`
	stmts, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	line := stmts[0].Lines[0]
	if line.AmountCents != 1000 || !line.Credit || line.EndToEndRef != "" || line.CounterpartyName != "" {
		t.Fatalf("line = %+v", line)
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
	if got := parseDate("2026-10-28T08:00:00"); !got.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parseDate(datetime) = %v", got)
	}
	if got := parseDate("nonsense"); !got.IsZero() {
		t.Errorf("parseDate(nonsense) = %v, want zero", got)
	}
	if got := parseDate(""); !got.IsZero() {
		t.Errorf("parseDate(empty) = %v, want zero", got)
	}
}

// TestParseMatchesBankstmtShape is a compile-time guard that Parse feeds the same
// shared shape as the other importers.
func TestParseMatchesBankstmtShape(t *testing.T) {
	var _ []bankstmt.Statement = mustParse(t)
}

func mustParse(t *testing.T) []bankstmt.Statement {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden_camt054.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return stmts
}

// TestGoldenValidatesAgainstVendorXSD validates the golden camt.054.001.08 file against the
// vendor schema when ERP_XSD_CAMT054 points at it. The schema is proprietary and is not
// committed, so the check skips when the variable is unset (CI stays green); set it
// to the extracted XSD to actually validate.
func TestGoldenValidatesAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_CAMT054", filepath.Join("testdata", "golden_camt054.xml"))
}
