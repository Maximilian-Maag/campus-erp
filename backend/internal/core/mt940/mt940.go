// Package mt940 parses SWIFT MT940 bank statements (Kontoauszüge).
//
// The layout and the field semantics follow the reference implementation parser
// (the reference parser.mt940.Field61 / MT940Record): value date YYMMDD, an
// optional MMDD booking date, a debit/credit indicator (C/D, or the reversal
// forms RC/RD), an optional currency, the amount, an "N" separator, the
// three-character posting key, the client reference and an optional bank
// reference after "//".
package mt940

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/bankstmt"
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

// Line is one :61: statement line (one booking). It is the shared parsed shape.
type Line = bankstmt.Line

// Statement is one MT940 statement (one account, one statement number). It is
// the shared parsed shape produced by every statement parser.
type Statement = bankstmt.Statement

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
				apply86(&cur.Lines[len(cur.Lines)-1], f.value)
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

// dkTags are the DK ("Deutsche Kreditwirtschaft") structured :86: markers the
// productive parser recognises (MT940.REF_*).
var dkTags = []string{"EREF+", "KREF+", "MREF+", "CRED+", "DEBT+", "COAM+", "SVWZ+", "ABWA+", "ABWE+"}

// numbered86 matches the numbered subfield convention "?NN".
var numbered86 = regexp.MustCompile(`\?[0-9]{2}`)

// apply86 parses a :86: field into the last line: the purpose plus any
// structured references and counterparty data it carries.
func apply86(line *Line, text string) {
	purpose, endToEnd, name, iban, bic := parse86(text)
	if purpose != "" {
		if line.Purpose == "" {
			line.Purpose = purpose
		} else {
			line.Purpose += " " + purpose
		}
	}
	if endToEnd != "" {
		line.EndToEndRef = endToEnd
	}
	if name != "" {
		line.CounterpartyName = name
	}
	if iban != "" {
		line.CounterpartyIBAN = iban
	}
	if bic != "" {
		line.CounterpartyBIC = bic
	}
}

// parse86 parses a :86: value in either the numbered ("?NN") or the DK ("TAG+")
// convention; anything else is free text and becomes the purpose.
func parse86(text string) (purpose, endToEnd, name, iban, bic string) {
	text = strings.TrimSpace(text)
	if numbered86.MatchString(text) {
		return parse86Numbered(text)
	}
	if idx := firstDKMarker(text); idx >= 0 {
		return parse86DK(text, idx)
	}
	return text, "", "", "", ""
}

// parse86Numbered parses the numbered convention: ?30 routing/BIC, ?31
// account/IBAN, ?32/?33 account holder, ?34 purpose.
func parse86Numbered(text string) (purpose, endToEnd, name, iban, bic string) {
	locs := numbered86.FindAllStringIndex(text, -1)
	var nameParts []string
	for k, loc := range locs {
		code := text[loc[0]+1 : loc[1]]
		valStart := loc[1]
		valEnd := len(text)
		if k+1 < len(locs) {
			valEnd = locs[k+1][0]
		}
		val := strings.TrimSpace(text[valStart:valEnd])
		switch code {
		case "30":
			bic = val
		case "31":
			iban = val
		case "32", "33":
			nameParts = append(nameParts, val)
		case "34":
			purpose = val
		}
	}
	return purpose, "", strings.TrimSpace(strings.Join(nameParts, " ")), iban, bic
}

// parse86DK parses the DK "TAG+" marker convention; idx is the first marker.
func parse86DK(text string, idx int) (purpose, endToEnd, name, iban, bic string) {
	purpose = strings.TrimSpace(text[:idx])
	for _, tag := range dkTags {
		pos := strings.Index(text, tag)
		if pos < 0 {
			continue
		}
		val := valueUntilNextTag(text[pos+len(tag):])
		switch tag {
		case "EREF+":
			endToEnd = val
		case "SVWZ+":
			purpose = val
		case "ABWA+", "ABWE+":
			if name == "" {
				name = val
			}
		}
	}
	return purpose, endToEnd, name, iban, bic
}

// valueUntilNextTag returns the text up to the next DK marker, or to the end.
func valueUntilNextTag(s string) string {
	end := len(s)
	for _, tag := range dkTags {
		if p := strings.Index(s, tag); p >= 0 && p < end {
			end = p
		}
	}
	return strings.TrimSpace(s[:end])
}

// firstDKMarker returns the index of the earliest DK marker in text, or -1.
func firstDKMarker(text string) int {
	best := -1
	for _, tag := range dkTags {
		if p := strings.Index(text, tag); p >= 0 && (best < 0 || p < best) {
			best = p
		}
	}
	return best
}

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
