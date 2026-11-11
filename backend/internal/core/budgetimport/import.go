// Package budgetimport reads a budget file — Mittelquellen, Bewirtschaftungsstellen and
// Haushaltsstellen in one pass — into rows the budget module imports (docs/10).
//
// The layout is DATA, not code. The vendor's budget_import layout is unpublished (docs/08 §3, open
// question Q8), so the layout shipped here is the target system's own: a Layout value names the
// delimiter, whether the file carries a header, and which column carries which field, and
// testdata/budget-import-v1.csv is that layout written down. Replacing it for the vendor's export
// means replacing the Layout value and the sample, not this file's logic.
package budgetimport

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Field names what a column means. A Layout maps each field to a column index; a field the layout
// does not carry is simply absent, which is how an optional column stays optional.
type Field int

const (
	// FieldSourceCode is the Mittelquelle's code: the identity an import and a person share.
	FieldSourceCode Field = iota
	// FieldSourceName is the Mittelquelle's name, used when it is created.
	FieldSourceName
	// FieldSourceChapter is the Haushaltsstelle's chapter, optional (a third-party funder has none).
	FieldSourceChapter
	// FieldCenterCode is the Bewirtschaftungsstelle's code.
	FieldCenterCode
	// FieldCenterName is the Bewirtschaftungsstelle's name, used when it is created.
	FieldCenterName
	// FieldYear is the Haushaltsjahr.
	FieldYear
	// FieldChapter is the position's Kapitel.
	FieldChapter
	// FieldTitle is the position's Titel.
	FieldTitle
	// FieldDescription is what the position is for.
	FieldDescription
	// FieldAllocated is the Ansatz, in euros with German decimals ("1.234,56").
	FieldAllocated
	// fieldCount is the number of fields a Layout can name.
	fieldCount
)

// Layout is a budget file's shape: how the columns are separated, whether the first line names
// them, and which column carries which field.
type Layout struct {
	// ID names the layout; it is what an operator states and what the audit records.
	ID string
	// Delimiter separates the columns.
	Delimiter string
	// Header says the first non-blank line names the columns and is not data.
	Header bool
	// Columns maps a field to its column index, counting from zero.
	Columns map[Field]int
}

// StandardLayout is the layout this build reads. It is the target system's own: semicolon
// separated, a header line, and the two codes carrying their names.
var StandardLayout = Layout{
	ID:        "budget-import/v1",
	Delimiter: ";",
	Header:    true,
	Columns: map[Field]int{
		FieldSourceCode:    0,
		FieldSourceName:    1,
		FieldSourceChapter: 2,
		FieldCenterCode:    3,
		FieldCenterName:    4,
		FieldYear:          5,
		FieldChapter:       6,
		FieldTitle:         7,
		FieldDescription:   8,
		FieldAllocated:     9,
	},
}

// Layouts lists the layouts this build can read, so a caller can offer them and refuse the rest.
func Layouts() []Layout { return []Layout{StandardLayout} }

// LayoutByID returns the layout with that id. An empty id is the standard layout, so the common
// call does not have to name it.
func LayoutByID(id string) (Layout, error) {
	if strings.TrimSpace(id) == "" {
		return StandardLayout, nil
	}
	for _, l := range Layouts() {
		if l.ID == id {
			return l, nil
		}
	}
	return Layout{}, fmt.Errorf("%w: %q", ErrUnknownLayout, id)
}

// Row is one line of a budget file: a position, and the source and centre it draws on and is
// administered by. The two code pairs are empty when the file does not name them.
type Row struct {
	Line           int
	SourceCode     string
	SourceName     string
	SourceChapter  string
	CenterCode     string
	CenterName     string
	Year           int
	Chapter        string
	Title          string
	Description    string
	AllocatedCents int64
}

var (
	// ErrUnknownLayout is returned for a layout id this build cannot read.
	ErrUnknownLayout = errors.New("budgetimport: unknown layout")
	// ErrEmpty is returned for a file with nothing to import.
	ErrEmpty = errors.New("budgetimport: nothing to import")
	// ErrBadRow is returned for a line that cannot be read. It names the line, because a budget
	// file is hand-edited and "row 37 is wrong" is the only useful thing to say.
	ErrBadRow = errors.New("budgetimport: cannot read row")
)

// ErrRow wraps a message with the line it belongs to, so every refusal reads the same way.
func ErrRow(line int, format string, args ...any) error {
	return fmt.Errorf("%w %d: %s", ErrBadRow, line, fmt.Sprintf(format, args...))
}

// machineDecimal says an amount's single dot is a decimal mark rather than a thousands separator:
// exactly one dot, and one or two digits after it.
func machineDecimal(s string) bool {
	whole, frac, found := strings.Cut(s, ".")
	if !found || strings.Contains(frac, ".") {
		return false
	}
	return whole != "" && len(frac) >= 1 && len(frac) <= 2
}

// Parse reads a budget file into rows. It reads the whole file before returning any of it and
// refuses on the first line it cannot read, so a caller that writes what it is given never writes
// half a budget.
func Parse(layout Layout, content string) ([]Row, error) {
	if layout.Delimiter == "" {
		return nil, fmt.Errorf("%w: no delimiter", ErrUnknownLayout)
	}
	if utf8.RuneCountInString(layout.Delimiter) != 1 {
		return nil, fmt.Errorf("%w: delimiter %q is not one character", ErrUnknownLayout, layout.Delimiter)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	rows := make([]Row, 0, len(lines))
	seen := make(map[string]int, len(lines))
	header := layout.Header
	for i, raw := range lines {
		line := i + 1
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if header {
			header = false
			continue
		}
		row, err := layout.parseLine(line, raw)
		if err != nil {
			return nil, err
		}
		key := row.key()
		if first, dup := seen[key]; dup {
			return nil, ErrRow(line, "the position %s is already on line %d", key, first)
		}
		seen[key] = line
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, ErrEmpty
	}
	return rows, nil
}

// key identifies a position: the year, chapter and title together are what it is.
func (r Row) key() string {
	return fmt.Sprintf("%d/%s/%s", r.Year, r.Chapter, r.Title)
}

// parseLine reads one data line. Every field the layout names must be there, and the fields that
// make a row mean something — the year, chapter, title and Ansatz — must be readable.
func (l Layout) parseLine(line int, raw string) (Row, error) {
	cells := strings.Split(raw, l.Delimiter)
	row := Row{Line: line}
	cell := func(f Field) (string, bool) {
		i, ok := l.Columns[f]
		if !ok {
			return "", true
		}
		if i >= len(cells) {
			return "", false
		}
		return strings.TrimSpace(cells[i]), true
	}
	for f := Field(0); f < fieldCount; f++ {
		v, present := cell(f)
		if !present {
			return Row{}, ErrRow(line, "the file has no column %d in this row", l.Columns[f]+1)
		}
		switch f {
		case FieldSourceCode:
			row.SourceCode = v
		case FieldSourceName:
			row.SourceName = v
		case FieldSourceChapter:
			row.SourceChapter = v
		case FieldCenterCode:
			row.CenterCode = v
		case FieldCenterName:
			row.CenterName = v
		case FieldYear:
			year, err := strconv.Atoi(v)
			if err != nil {
				return Row{}, ErrRow(line, "the year %q is not a number", v)
			}
			row.Year = year
		case FieldChapter:
			row.Chapter = v
		case FieldTitle:
			row.Title = v
		case FieldDescription:
			row.Description = v
		case FieldAllocated:
			cents, err := ParseEuros(v)
			if err != nil {
				return Row{}, ErrRow(line, "the Ansatz %q: %s", v, err)
			}
			row.AllocatedCents = cents
		default:
			// Unreachable for the fields this build knows; a layout carrying another field cannot be
			// read into a Row, and reading it as "no such field" would silently drop a column.
			return Row{}, ErrRow(line, "the layout names a field this build cannot read")
		}
	}
	return row, row.validate()
}

// validate refuses a row that is not a position, and reports which line it is on.
func (r Row) validate() error {
	if r.Year < 2000 || r.Year > 2200 {
		return ErrRow(r.Line, "the year %d is not a Haushaltsjahr", r.Year)
	}
	if r.Chapter == "" {
		return ErrRow(r.Line, "the Kapitel is missing")
	}
	if r.Title == "" {
		return ErrRow(r.Line, "the Titel is missing")
	}
	if r.AllocatedCents < 0 {
		return ErrRow(r.Line, "the Ansatz %d is negative", r.AllocatedCents)
	}
	if r.SourceCode != "" && r.SourceName == "" {
		return ErrRow(r.Line, "the Mittelquelle %s has no name", r.SourceCode)
	}
	if r.CenterCode != "" && r.CenterName == "" {
		return ErrRow(r.Line, "the Bewirtschaftungsstelle %s has no name", r.CenterCode)
	}
	return nil
}

// ParseEuros reads an amount in euros into cents, accepting both forms a budget file carries: the
// German one a person writes ("1.234,56") and the plain one a machine writes ("1234.56"). A comma
// is the decimal mark and every dot is a thousands separator; with no comma, a **single** dot
// followed by one or two digits is a decimal mark (a machine's file) and anything else is a
// thousands separator (a person's). More than two decimals, or a negative amount, is refused.
func ParseEuros(s string) (int64, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return 0, nil
	}
	if strings.HasPrefix(in, "-") {
		return 0, errors.New("the amount is negative")
	}
	if strings.Contains(in, ",") {
		in = strings.ReplaceAll(in, ".", "")
	} else if !machineDecimal(in) {
		in = strings.ReplaceAll(in, ".", "")
	}
	in = strings.ReplaceAll(in, ",", ".")
	whole, frac, hasFrac := strings.Cut(in, ".")
	digits := whole
	if hasFrac {
		if len(frac) > 2 {
			return 0, errors.New("the amount has more than two decimals")
		}
		for len(frac) < 2 {
			frac += "0"
		}
		digits += frac
	} else {
		digits += "00"
	}
	if digits == "" {
		return 0, errors.New("the amount is empty")
	}
	cents, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, errors.New("the amount is not a number")
	}
	return cents, nil
}
