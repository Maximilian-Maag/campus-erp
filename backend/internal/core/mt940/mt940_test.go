package mt940

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "mt940_sample.sta"))
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
	if s.Reference != "STMT-2026-0001" || s.Account != "DE02120300000000202051" {
		t.Fatalf("header = %+v", s)
	}
	if s.Number != 1 || s.Page != 1 {
		t.Fatalf("statement no = %d/%d, want 1/1", s.Number, s.Page)
	}
	if s.Currency != "EUR" {
		t.Fatalf("currency = %q, want EUR", s.Currency)
	}
	if s.OpeningBalanceCents != 1000000 || s.ClosingBalanceCents != 1110001 {
		t.Fatalf("balances = %d .. %d", s.OpeningBalanceCents, s.ClosingBalanceCents)
	}
	if got := s.OpeningDate; !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("opening date = %v", got)
	}
	if len(s.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(s.Lines))
	}

	l0 := s.Lines[0]
	if !l0.Credit || l0.Return || l0.AmountCents != 125000 || l0.PostingKey != "051" {
		t.Fatalf("line 0 = %+v", l0)
	}
	if l0.ClientRef != "NONREF" || l0.BankRef != "BANKREF1" || l0.Purpose != "Stromabschlag Oktober" {
		t.Fatalf("line 0 refs = %+v", l0)
	}
	if !l0.ValueDate.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		!l0.BookingDate.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("line 0 dates = %+v", l0)
	}

	l1 := s.Lines[1]
	if l1.Credit || l1.AmountCents != 4999 || l1.ClientRef != "AO-2026-0002" {
		t.Fatalf("line 1 = %+v", l1)
	}

	l2 := s.Lines[2]
	if !l2.Return || l2.Credit || l2.AmountCents != 10000 || l2.PostingKey != "059" {
		t.Fatalf("line 2 = %+v", l2)
	}
}

func TestParseMultipleStatements(t *testing.T) {
	text := ":20:A\n:25:DE1\n:28C:1/1\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n" +
		":20:B\n:25:DE2\n:28C:2/1\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n"
	stmts, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts) != 2 || stmts[0].Reference != "A" || stmts[1].Reference != "B" {
		t.Fatalf("statements = %+v", stmts)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"only header", ":20:A\n", ErrEmpty},
		{"line before balance", ":20:A\n:61:260101C1,00N051X\n", ErrLineWithoutBalance},
		{"bad amount", ":20:A\n:60F:C260101EUR0,00\n:61:260101C1,0X0N051X\n", ErrBadAmount},
		{"bad indicator", ":20:A\n:60F:C260101EUR0,00\n:61:260101X1,00N051X\n", ErrBadIndicator},
		{"bad date", ":20:A\n:60F:CXX0101EUR0,00\n", ErrBadDate},
		{"bad balance sign", ":20:A\n:60F:X260101EUR0,00\n", ErrBadBalance},
		{"short balance", ":20:A\n:60F:C26\n", ErrBadBalance},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.in); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"1250,00", 125000, false},
		{"49,99", 4999, false},
		{"0,01", 1, false},
		{"1.234,56", 123456, false},
		{"100", 10000, false},
		{"100,5", 10050, false},
		{"100,999", 10099, false},
		{"-5,00", -500, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := parseAmount(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("parseAmount(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAmount(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseAmount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDate(t *testing.T) {
	// Two-digit year resolves against the reference century.
	got, err := parseDate("261001", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate(261001) = %v, %v", got, err)
	}
	// Eight-digit form.
	got, err = parseDate("20261001", time.Time{})
	if err != nil || !got.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("parseDate(20261001) = %v, %v", got, err)
	}
	if _, err := parseDate("26", time.Time{}); err == nil {
		t.Fatal("short date must fail")
	}
	if _, err := parseDate("XXXXXX", time.Time{}); err == nil {
		t.Fatal("non-numeric date must fail")
	}
	// A zero reference resolves against the current century.
	if _, err := parseDate("260101", time.Time{}); err != nil {
		t.Fatalf("zero ref: %v", err)
	}
}

// TestParseNegativeBalances covers the debit (D) balance branches.
func TestParseNegativeBalances(t *testing.T) {
	stmts, err := Parse(":20:A\n:25:DE1\n:28C:1/1\n:60F:D260101EUR500,00\n:62M:D260131EUR300,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if stmts[0].OpeningBalanceCents != -50000 || stmts[0].ClosingBalanceCents != -30000 {
		t.Fatalf("balances = %d .. %d, want -50000 .. -30000", stmts[0].OpeningBalanceCents, stmts[0].ClosingBalanceCents)
	}
	if stmts[0].Currency != "EUR" {
		t.Fatalf("currency = %q, want EUR", stmts[0].Currency)
	}
}

// TestParseBalanceWithoutCurrency covers the "no currency in balance" branch.
func TestParseBalanceWithoutCurrency(t *testing.T) {
	stmts, err := Parse(":20:A\n:60F:C260101500,00\n:62F:C260101700,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if stmts[0].OpeningBalanceCents != 50000 || stmts[0].Currency != "" {
		t.Fatalf("statement = %+v", stmts[0])
	}
}

// TestParseBalanceLengthBoundary covers the minimum balance length (8).
func TestParseBalanceLengthBoundary(t *testing.T) {
	// Exactly eight characters is accepted.
	if _, err := Parse(":20:A\n:60F:C2601010\n:62F:C2601010\n"); err != nil {
		t.Fatalf("length-8 balance must be accepted: %v", err)
	}
	// Seven characters is rejected.
	if _, err := Parse(":20:A\n:60F:C260101\n"); !errors.Is(err, ErrBadBalance) {
		t.Fatalf("length-7 balance: %v", err)
	}
}

// TestParsePurposeJoinsAndIgnoresOrphans covers the :86: branches.
func TestParsePurposeJoinsAndIgnoresOrphans(t *testing.T) {
	text := ":20:A\n:60F:C260101EUR0,00\n:86:orphan\n:61:260101C1,00N051REF\n:86:first\n:86:second\n:62F:C260101EUR0,00\n"
	stmts, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(stmts[0].Lines) != 1 || stmts[0].Lines[0].Purpose != "first second" {
		t.Fatalf("purpose = %q, want 'first second'", stmts[0].Lines[0].Purpose)
	}
}

// TestParseLineCurrency covers the optional currency letter on a line.
func TestParseLineCurrency(t *testing.T) {
	// "R" means EUR.
	stmts, err := Parse(":20:A\n:60F:C260101EUR0,00\n:61:2601011001CR1250,00N051NONREF\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := stmts[0].Lines[0].Currency; got != "EUR" {
		t.Fatalf("currency = %q, want EUR", got)
	}
	// Any other letter is taken literally.
	stmts, err = Parse(":20:A\n:60F:C260101EUR0,00\n:61:2601011001CX1250,00N051NONREF\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := stmts[0].Lines[0].Currency; got != "X" {
		t.Fatalf("currency = %q, want X", got)
	}
}

// TestParseLineNoBankRef covers a client reference without the "//" separator.
func TestParseLineNoBankRef(t *testing.T) {
	stmts, err := Parse(":20:A\n:60F:C260101EUR0,00\n:61:260101C1250,00N051NONREF\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	l := stmts[0].Lines[0]
	if l.ClientRef != "NONREF" || l.BankRef != "" {
		t.Fatalf("line = %+v", l)
	}
}

// TestParseLineErrors covers the line-level error branches.
func TestParseLineErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want error
	}{
		{"too short", ":20:A\n:60F:C260101EUR0,00\n:61:2610\n", ErrBadDate},
		{"no indicator", ":20:A\n:60F:C260101EUR0,00\n:61:261001\n", ErrBadIndicator},
		{"R without indicator", ":20:A\n:60F:C260101EUR0,00\n:61:261001R\n", ErrBadIndicator},
		{"no N separator", ":20:A\n:60F:C260101EUR0,00\n:61:261001C1250,00\n", ErrBadAmount},
		{"short posting key", ":20:A\n:60F:C260101EUR0,00\n:61:261001C1250,00N05\n", ErrBadAmount},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.in); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestParseAmountDotDecimal(t *testing.T) {
	// A dot is treated as the decimal separator when no comma is present.
	got, err := parseAmount("1234.56")
	if err != nil || got != 123456 {
		t.Fatalf("parseAmount(1234.56) = %d, %v", got, err)
	}
}

// TestParseAmountLeadingSeparator covers a leading decimal separator.
func TestParseAmountLeadingSeparator(t *testing.T) {
	if got, err := parseAmount(",50"); err != nil || got != 50 {
		t.Fatalf("parseAmount(,50) = %d, %v", got, err)
	}
	if got, err := parseAmount(".50"); err != nil || got != 50 {
		t.Fatalf("parseAmount(.50) = %d, %v", got, err)
	}
}

// TestParseLineTrailingAndEmptyRef covers a line ending right after the posting
// key, and one whose client reference is empty but carries a bank reference.
func TestParseLineTrailingAndEmptyRef(t *testing.T) {
	// Ends exactly after the three-character posting key.
	stmts, err := Parse(":20:A\n:60F:C260101EUR0,00\n:61:260101C1250,00N051\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := stmts[0].Lines[0].PostingKey; got != "051" {
		t.Fatalf("posting key = %q, want 051", got)
	}
	// Client reference empty, bank reference present.
	stmts, err = Parse(":20:A\n:60F:C260101EUR0,00\n:61:260101C1250,00N051//BANKREF\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	l := stmts[0].Lines[0]
	if l.ClientRef != "" || l.BankRef != "BANKREF" {
		t.Fatalf("line = %+v", l)
	}
}

func TestTokenizeSkipsMalformedTag(t *testing.T) {
	// A line starting with ":" but with no second ":" is not a tag.
	stmts, err := Parse(":20:A\n:\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if stmts[0].Reference != "A" {
		t.Fatalf("reference = %q", stmts[0].Reference)
	}
}

func TestIsDigitAndIsAlpha(t *testing.T) {
	for _, s := range []string{"0", "5", "9"} {
		if !isDigit(s) {
			t.Errorf("isDigit(%q) = false", s)
		}
	}
	for _, s := range []string{"", "12", "a", "-"} {
		if isDigit(s) {
			t.Errorf("isDigit(%q) = true", s)
		}
	}
	for _, s := range []string{"A", "Z", "z", "EUR"} {
		if !isAlpha(s) {
			t.Errorf("isAlpha(%q) = false", s)
		}
	}
	for _, s := range []string{"", "A1", "!", "1"} {
		if isAlpha(s) {
			t.Errorf("isAlpha(%q) = true", s)
		}
	}
}
