// Package mt940 parses SWIFT MT940 bank statements (Kontoauszüge).
//
// The layout and the field semantics follow the productive HISinOne parser
// (rm.fia.payments.datex.mt940.Field61 / MT940Record): value date YYMMDD, an
// optional MMDD booking date, a debit/credit indicator (C/D, or the reversal
// forms RC/RD), an optional currency, the amount, an "N" separator, the
// three-character posting key, the client reference and an optional bank
// reference after "//".
package mt940

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse errors. They are values so callers can match them with errors.Is.
var (
	ErrEmpty              = errors.New("mt940: empty statement")
	ErrBadAmount          = errors.New("mt940: malformed amount")
	ErrBadDate            = errors.New("mt940: malformed date")
	ErrBadIndicator       = errors.New("mt940: malformed debit/credit indicator")
	ErrBadBalance         = errors.New("mt940: malformed balance")
	ErrLineWithoutBalance = errors.New("mt940: statement line before any opening balance")
)

// Line is one :61: statement line (one booking).
type Line struct {
	ValueDate   time.Time
	BookingDate time.Time
	Credit      bool
	Return      bool // RC/RD, or a return posting key (009/059)
	AmountCents int64
	Currency    string
	PostingKey  string
	ClientRef   string
	BankRef     string
	Purpose     string
}

// Statement is one MT940 statement (one account, one statement number).
type Statement struct {
	Reference           string
	Account             string
	Number              int
	Page                int
	Currency            string
	OpeningBalanceCents int64
	OpeningDate         time.Time
	ClosingBalanceCents int64
	ClosingDate         time.Time
	Lines               []Line
}

type field struct {
	tag   string
	value string
}

// tokenize splits the MT940 text into tagged fields, joining continuation lines.
func tokenize(text string) []field {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var fields []field
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\n")
		// Skip the SWIFT header block ("{1:...}") and the statement terminator
		// ("-" / "-}").
		if strings.HasPrefix(line, "{") || line == "-" || line == "-}" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			rest := line[1:]
			idx := strings.Index(rest, ":")
			if idx < 0 {
				continue
			}
			fields = append(fields, field{tag: rest[:idx], value: rest[idx+1:]})
			continue
		}
		if len(fields) == 0 {
			continue
		}
		last := &fields[len(fields)-1]
		last.value += "\n" + line
	}
	return fields
}

// Parse parses a whole MT940 file into its statements.
func Parse(text string) ([]Statement, error) {
	fields := tokenize(text)
	if len(fields) == 0 {
		return nil, ErrEmpty
	}
	var out []Statement
	var cur *Statement
	flush := func() {
		// A statement with no opening balance and no lines carries no data.
		if cur != nil && (len(cur.Lines) > 0 || !cur.OpeningDate.IsZero()) {
			out = append(out, *cur)
		}
	}
	for _, f := range fields {
		switch f.tag {
		case "20":
			flush()
			cur = &Statement{Reference: strings.TrimSpace(f.value)}
		case "25":
			if cur == nil {
				cur = &Statement{}
			}
			cur.Account = strings.TrimSpace(f.value)
		case "28C":
			if cur == nil {
				cur = &Statement{}
			}
			cur.Number, cur.Page = parseStatementNo(f.value)
		case "60F", "60M":
			if cur == nil {
				cur = &Statement{}
			}
			dc, amount, date, ccy, err := parseBalance(f.value)
			if err != nil {
				return nil, err
			}
			cur.OpeningBalanceCents = amount
			if !dc {
				cur.OpeningBalanceCents = -amount
			}
			cur.OpeningDate = date
			if ccy != "" {
				cur.Currency = ccy
			}
		case "62F", "62M":
			if cur == nil {
				cur = &Statement{}
			}
			dc, amount, date, ccy, err := parseBalance(f.value)
			if err != nil {
				return nil, err
			}
			cur.ClosingBalanceCents = amount
			if !dc {
				cur.ClosingBalanceCents = -amount
			}
			cur.ClosingDate = date
			if ccy != "" {
				cur.Currency = ccy
			}
		case "61":
			if cur == nil || cur.OpeningDate.IsZero() {
				return nil, ErrLineWithoutBalance
			}
			line, err := parseLine(f.value, cur.OpeningDate)
			if err != nil {
				return nil, err
			}
			cur.Lines = append(cur.Lines, line)
		case "86":
			if cur != nil && len(cur.Lines) > 0 {
				purpose := strings.TrimSpace(f.value)
				if cur.Lines[len(cur.Lines)-1].Purpose == "" {
					cur.Lines[len(cur.Lines)-1].Purpose = purpose
				} else {
					cur.Lines[len(cur.Lines)-1].Purpose += " " + purpose
				}
			}
		}
	}
	flush()
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

// parseStatementNo parses a :28C: value like "00001/002" into number and page.
func parseStatementNo(v string) (number, page int) {
	v = strings.TrimSpace(v)
	parts := strings.SplitN(v, "/", 2)
	number = atoiSafe(parts[0])
	if len(parts) == 2 {
		page = atoiSafe(parts[1])
	}
	return number, page
}

// parseBalance parses a :60:/:62: value: D/C + YYMMDD + [currency] + amount.
func parseBalance(v string) (credit bool, amountCents int64, date time.Time, currency string, err error) {
	v = strings.TrimSpace(v)
	if len(v) < 8 {
		return false, 0, time.Time{}, "", ErrBadBalance
	}
	switch v[0] {
	case 'C':
		credit = true
	case 'D':
		credit = false
	default:
		return false, 0, time.Time{}, "", ErrBadBalance
	}
	date, err = parseDate(v[1:7], time.Time{})
	if err != nil {
		return false, 0, time.Time{}, "", err
	}
	rest := v[7:]
	if len(rest) >= 3 && isAlpha(rest[:3]) {
		currency = rest[:3]
		rest = rest[3:]
	}
	amountCents, err = parseAmount(rest)
	if err != nil {
		return false, 0, time.Time{}, "", err
	}
	return credit, amountCents, date, currency, nil
}

// parseLine parses a :61: statement line.
func parseLine(v string, ref time.Time) (Line, error) {
	v = strings.TrimRight(v, "\n")
	if len(v) < 6 {
		return Line{}, ErrBadDate
	}
	var line Line
	valuta, err := parseDate(v[0:6], ref)
	if err != nil {
		return Line{}, err
	}
	line.ValueDate = valuta

	idx := 6
	// Optional 4-digit MMDD booking date, using the value date's year.
	if idx < len(v) && isDigit(v[idx:idx+1]) {
		if idx+4 <= len(v) {
			if bd, err := parseBookingDate(v[idx:idx+4], valuta); err == nil {
				line.BookingDate = bd
			}
		}
		idx += 4
	}

	// Debit/credit indicator: C, D, or the reversal forms RC / RD.
	if idx >= len(v) {
		return Line{}, ErrBadIndicator
	}
	ind := v[idx : idx+1]
	idx++
	if ind == "R" {
		if idx >= len(v) {
			return Line{}, ErrBadIndicator
		}
		ind = "R" + v[idx:idx+1]
		idx++
	}
	switch ind {
	case "C":
		line.Credit = true
	case "D":
		line.Credit = false
	case "RC":
		line.Credit = true
		line.Return = true
	case "RD":
		line.Credit = false
		line.Return = true
	default:
		return Line{}, ErrBadIndicator
	}

	// Optional currency letter (R means EUR).
	if idx < len(v) && isAlpha(v[idx:idx+1]) {
		if v[idx] == 'R' {
			line.Currency = "EUR"
		} else {
			line.Currency = v[idx : idx+1]
		}
		idx++
	}

	// Amount, terminated by the literal "N".
	end := strings.Index(v[idx:], "N")
	if end < 0 {
		return Line{}, ErrBadAmount
	}
	amount, err := parseAmount(v[idx : idx+end])
	if err != nil {
		return Line{}, err
	}
	line.AmountCents = amount
	idx += end + 1

	// Posting key: three characters.
	if idx+3 > len(v) {
		return Line{}, ErrBadAmount
	}
	line.PostingKey = v[idx : idx+3]
	idx += 3

	// Client reference up to "//"; bank reference after it.
	rest := v[idx:]
	line.ClientRef = strings.TrimSpace(rest)
	if c := strings.Index(rest, "//"); c >= 0 {
		line.ClientRef = strings.TrimSpace(rest[:c])
		line.BankRef = strings.TrimSpace(rest[c+2:])
	}
	// Return postings: the reversal indicator, or the return posting keys the
	// productive parser treats as back-postings (009 direct-debit return,
	// 059 payment-out return).
	if line.PostingKey == "009" || line.PostingKey == "059" {
		line.Return = true
	}
	return line, nil
}

// parseAmount parses an MT940 amount. The decimal separator is a comma; a dot is
// a thousands separator (German convention). Without any separator the fraction
// is zero.
func parseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrBadAmount
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var intPart, fracPart string
	if i := strings.LastIndex(s, ","); i >= 0 {
		intPart = strings.ReplaceAll(s[:i], ".", "")
		fracPart = s[i+1:]
	} else if i := strings.LastIndex(s, "."); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
	} else {
		intPart = s
	}
	intPart = strings.ReplaceAll(intPart, " ", "")
	fracPart = strings.ReplaceAll(fracPart, " ", "")
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 2 {
		fracPart = fracPart[:2]
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	units, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrBadAmount
	}
	cents, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, ErrBadAmount
	}
	total := units*100 + cents
	if neg {
		total = -total
	}
	return total, nil
}

// parseDate parses an MT940 date. YYMMDD is resolved against the century of ref
// (or the current century when ref is zero). The digits must be numeric.
func parseDate(s string, ref time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) != 6 && len(s) != 8 {
		return time.Time{}, ErrBadDate
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return time.Time{}, fmt.Errorf("%w: %q", ErrBadDate, s)
		}
	}
	if len(s) == 8 {
		t, err := time.Parse("20060102", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %q", ErrBadDate, s)
		}
		return t, nil
	}
	base := ref
	if base.IsZero() {
		base = time.Now()
	}
	yy, _ := strconv.Atoi(s[0:2])
	century := (base.Year() / 100) * 100
	t, err := time.Parse("20060102", fmt.Sprintf("%04d%s", century+yy, s[2:]))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrBadDate, s)
	}
	return t, nil
}

// parseBookingDate parses a four-digit MMDD booking date against the value
// date's year.
func parseBookingDate(mmdds string, valuta time.Time) (time.Time, error) {
	return parseDate(fmt.Sprintf("%04d%s", valuta.Year(), mmdds), valuta)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func isDigit(s string) bool { return len(s) == 1 && s[0] >= '0' && s[0] <= '9' }

func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}
