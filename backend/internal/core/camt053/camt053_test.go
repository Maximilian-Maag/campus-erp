package camt053

import (
	"errors"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "camt053_sample.xml"))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	return string(b)
}

func TestParseSample(t *testing.T) {
	stmts, err := Parse(sample(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(stmts))
	}
	s := stmts[0]
	if s.Reference != "STMT-2026-0001" || s.Account != "DE02120300000000202051" || s.Currency != "EUR" {
		t.Fatalf("header = %+v", s)
	}
	if s.Number != 1 {
		t.Fatalf("number = %d, want 1", s.Number)
	}
	if s.OpeningBalanceCents != 1000000 || s.ClosingBalanceCents != 1110001 {
		t.Fatalf("balances = %d .. %d", s.OpeningBalanceCents, s.ClosingBalanceCents)
	}
	if !s.OpeningDate.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("opening date = %v", s.OpeningDate)
	}
	if len(s.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(s.Lines))
	}

	l0 := s.Lines[0]
	if !l0.Credit || l0.AmountCents != 125000 || l0.Currency != "EUR" {
		t.Fatalf("line 0 = %+v", l0)
	}
	if l0.EndToEndRef != "AO-2026-0001" || l0.ClientRef != "TX-1" {
		t.Fatalf("line 0 refs = %+v", l0)
	}
	if l0.CounterpartyName != "Stadtwerke Musterstadt" || l0.CounterpartyIBAN != "DE89370400440532013000" {
		t.Fatalf("line 0 counterparty = %+v", l0)
	}
	if l0.Purpose != "Stromabschlag Oktober" {
		t.Fatalf("line 0 purpose = %q", l0.Purpose)
	}

	l1 := s.Lines[1]
	if l1.Credit || l1.AmountCents != 4999 || l1.CounterpartyName != "Buchhandlung Am Markt" {
		t.Fatalf("line 1 = %+v", l1)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse("<not-xml"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed xml: %v", err)
	}
	if _, err := Parse("<Document></Document>"); !errors.Is(err, ErrNoStatement) {
		t.Fatalf("no statement: %v", err)
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1250.00", 125000},
		{"49.99", 4999},
		{"-100.50", -10050},
		{"0.01", 1},
		{"100", 10000},
		{"100.5", 10050},
		{"100.999", 10099},
		{".50", 50},
		{",50", 50},
		{"", 0},
		{"abc", 0},
	}
	for _, c := range cases {
		if got := parseAmount(c.in); got != c.want {
			t.Errorf("parseAmount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestParseDebitBalance covers the DBIT (negative) balance branch.
func TestParseDebitBalance(t *testing.T) {
	doc := `<Document><BkToCstmrStmt><Stmt><Id>S</Id><Acct><Id><IBAN>DE1</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">500.00</Amt><CdtDbtInd>DBIT</CdtDbtInd><Dt><Dt>2026-10-01</Dt></Dt></Bal>` +
		`</Stmt></BkToCstmrStmt></Document>`
	stmts, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if stmts[0].OpeningBalanceCents != -50000 {
		t.Fatalf("opening = %d, want -50000", stmts[0].OpeningBalanceCents)
	}
}

func TestParseDate(t *testing.T) {
	if got := parseDate("2026-10-01"); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate = %v", got)
	}
	if got := parseDate("2026-10-01T09:00:00"); !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate datetime = %v", got)
	}
	if got := parseDate(""); !got.IsZero() {
		t.Fatalf("parseDate empty = %v", got)
	}
	if got := parseDate("garbage"); !got.IsZero() {
		t.Fatalf("parseDate garbage = %v", got)
	}
}

// TestParseEntryWithoutDetails covers an Ntry with no NtryDtls.
func TestParseEntryWithoutDetails(t *testing.T) {
	doc := `<Document><BkToCstmrStmt><Stmt><Id>S</Id><Acct><Id><IBAN>DE1</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Ntry><Amt Ccy="EUR">10.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><BookgDt><Dt>2026-10-01</Dt></BookgDt><ValDt><Dt>2026-10-01</Dt></ValDt></Ntry>` +
		`</Stmt></BkToCstmrStmt></Document>`
	stmts, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts[0].Lines) != 1 || stmts[0].Lines[0].AmountCents != 1000 {
		t.Fatalf("line = %+v", stmts[0].Lines)
	}
}

// TestParseReturnFlag covers the only return signal a camt.053 gives: the return information
// block on the transaction detail. An entry carrying it was taken back by the bank, and the
// original reference travels in Refs/EndToEndId.
func TestParseReturnFlag(t *testing.T) {
	doc := func(details string) string {
		return `<Document><BkToCstmrStmt><Stmt><Id>S</Id>` +
			`<Acct><Id><IBAN>DE1</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
			`<Ntry><Amt Ccy="EUR">42.50</Amt><CdtDbtInd>DBIT</CdtDbtInd>` +
			`<BookgDt><Dt>2026-11-22</Dt></BookgDt><ValDt><Dt>2026-11-22</Dt></ValDt>` +
			`<NtryDtls><TxDtls>` + details + `</TxDtls></NtryDtls></Ntry>` +
			`</Stmt></BkToCstmrStmt></Document>`
	}
	for _, c := range []struct {
		name       string
		details    string
		wantReturn bool
	}{
		{name: "ordinary", details: `<Refs><EndToEndId>MAND-1</EndToEndId></Refs>`},
		{name: "return", details: `<Refs><EndToEndId>MAND-1</EndToEndId></Refs>` +
			`<RtrInf><Rsn><Cd>AC04</Cd></Rsn></RtrInf>`, wantReturn: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			stmts, err := Parse(doc(c.details))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			line := stmts[0].Lines[0]
			if line.Return != c.wantReturn {
				t.Fatalf("Return = %t, want %t (%+v)", line.Return, c.wantReturn, line)
			}
			if line.EndToEndRef != "MAND-1" {
				t.Fatalf("EndToEndRef = %q, want the original reference", line.EndToEndRef)
			}
		})
	}
}

// TestGoldenValidatesAgainstVendorXSD validates the golden camt.053.001.02/.08 file against the
// vendor schema when ERP_XSD_CAMT053 points at it. The schema is proprietary and is not
// committed, so the check skips when the variable is unset (CI stays green); set it
// to the extracted XSD to actually validate.
func TestGoldenValidatesAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_CAMT053", filepath.Join("testdata", "camt053_sample.xml"))
}
