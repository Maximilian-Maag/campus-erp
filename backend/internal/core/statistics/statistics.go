// Package statistics renders the per-state statistics exports a Statistisches
// Landesamt expects (requirements IR-4, FR-P9; docs/27).
//
// The design point: a state's file layout is DATA, not code. A Format names the
// fields it wants and how it separates them, and the Registry holds one entry per
// state. Adding a state is one registry entry and changing a state is editing its
// columns — the rendering pipeline is written once and never moves. These offices
// change their layouts on their own schedule, so that is the property that matters.
//
// The layouts seeded in registry.go are PROJECT-DEFINED PLACEHOLDERS, marked as
// such. The offices' real per-state layouts are not published (docs/open-questions
// Q8) and replace the placeholders without touching the pipeline.
package statistics

import (
	"encoding/csv"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Field is a canonical value an export can carry, independent of any state's layout.
type Field string

// The fields a layout may project. This set is the *only* code that changes when a
// state wants a value that does not exist yet; every other change is data.
const (
	FieldKey     Field = "key"
	FieldIncome  Field = "income"
	FieldExpense Field = "expense"
	FieldNet     Field = "net"
	FieldCount   Field = "count"
)

// known reports whether f is a field a layout can name.
func (f Field) known() bool {
	switch f {
	case FieldKey, FieldIncome, FieldExpense, FieldNet, FieldCount:
		return true
	}
	return false
}

// Column is one output column: which field it carries and its heading.
type Column struct {
	Field Field
	Title string
}

// Format is one state's file layout.
type Format struct {
	State      string   // stable key: the registry key and the job argument
	Name       string   // human name, for the register and the docs
	Delimiter  rune     // field separator: ';', ',' or tab
	Columns    []Column // the layout: which fields, in which order, under which headings
	Title      string   // optional caption line above the header (empty = none)
	Totals     bool     // append a totals row
	TotalLabel string   // the totals row's key cell (empty = "Summe")
}

// Validation errors. They are values so callers can match them with errors.Is.
var (
	ErrStateRequired    = errors.New("statistics: a format needs a state")
	ErrNameRequired     = errors.New("statistics: a format needs a name")
	ErrDelimiterInvalid = errors.New("statistics: the delimiter must be one of ';', ',' or tab")
	ErrColumnsRequired  = errors.New("statistics: a format needs at least one column")
	ErrColumnIncomplete = errors.New("statistics: a column needs a known field and a title")
	ErrDuplicateField   = errors.New("statistics: column fields must be unique")
	ErrEntryKeyRequired = errors.New("statistics: an entry needs a key")
	ErrUnknownKind      = errors.New("statistics: unknown entry kind")
)

// Validate checks that a format is usable.
func (f Format) Validate() error {
	if strings.TrimSpace(f.State) == "" {
		return ErrStateRequired
	}
	if strings.TrimSpace(f.Name) == "" {
		return ErrNameRequired
	}
	switch f.Delimiter {
	case ';', ',', '\t':
	default:
		return ErrDelimiterInvalid
	}
	if len(f.Columns) == 0 {
		return ErrColumnsRequired
	}
	seen := make(map[Field]bool, len(f.Columns))
	for _, c := range f.Columns {
		if !c.Field.known() || strings.TrimSpace(c.Title) == "" {
			return fmt.Errorf("%w: field=%q title=%q", ErrColumnIncomplete, c.Field, c.Title)
		}
		if seen[c.Field] {
			return fmt.Errorf("%w: %q", ErrDuplicateField, c.Field)
		}
		seen[c.Field] = true
	}
	return nil
}

// Record is one aggregated line of an export: a key and its figures.
type Record struct {
	Key          string
	IncomeCents  int64
	ExpenseCents int64
	Count        int
}

// NetCents is income minus expense.
func (r Record) NetCents() int64 { return r.IncomeCents - r.ExpenseCents }

// Kind is whether an entry is money in or money out.
type Kind string

// The two entry kinds.
const (
	Income  Kind = "income"
	Expense Kind = "expense"
)

// Entry is one figure to aggregate. It is deliberately free of any domain type, so
// this package depends on nothing but the shape of the data (the same seam the job
// kinds use).
type Entry struct {
	Key         string
	Kind        Kind
	AmountCents int64
}

// Group sums entries into one Record per key, ordered by key, so the same entries
// always produce the same file. An entry with no key or an unknown kind is refused
// rather than dropped — a bad mapping must fail loudly, not lose money silently.
func Group(entries []Entry) ([]Record, error) {
	byKey := make(map[string]*Record, len(entries))
	for _, e := range entries {
		key := strings.TrimSpace(e.Key)
		if key == "" {
			return nil, ErrEntryKeyRequired
		}
		rec, ok := byKey[key]
		if !ok {
			rec = &Record{Key: key}
			byKey[key] = rec
		}
		switch e.Kind {
		case Income:
			rec.IncomeCents += e.AmountCents
		case Expense:
			rec.ExpenseCents += e.AmountCents
		default:
			return nil, fmt.Errorf("%w: %q", ErrUnknownKind, e.Kind)
		}
		rec.Count++
	}
	out := make([]Record, 0, len(byKey))
	for _, rec := range byKey {
		out = append(out, *rec)
	}
	slices.SortFunc(out, func(a, b Record) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

// Render writes the format's file: an optional caption, a header row of the column
// titles, one row per record and, where the format asks for one, a totals row.
func Render(f Format, records []Record) (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	if caption := strings.TrimSpace(f.Title); caption != "" {
		b.WriteString(caption)
		b.WriteString("\n")
	}
	w := csv.NewWriter(&b)
	w.Comma = f.Delimiter
	if err := w.Write(header(f)); err != nil {
		return "", fmt.Errorf("statistics: header: %w", err)
	}
	for _, rec := range records {
		if err := w.Write(project(f, rec)); err != nil {
			return "", fmt.Errorf("statistics: row: %w", err)
		}
	}
	if f.Totals {
		if err := w.Write(totals(f, records)); err != nil {
			return "", fmt.Errorf("statistics: totals: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("statistics: %w", err)
	}
	return b.String(), nil
}

// header is the column titles, in layout order.
func header(f Format) []string {
	titles := make([]string, 0, len(f.Columns))
	for _, c := range f.Columns {
		titles = append(titles, c.Title)
	}
	return titles
}

// project renders one record through the format's columns.
func project(f Format, rec Record) []string {
	row := make([]string, 0, len(f.Columns))
	for _, c := range f.Columns {
		row = append(row, value(c.Field, rec))
	}
	return row
}

// totals is the format's final row: its key cell and the sums of the records.
func totals(f Format, records []Record) []string {
	sum := Record{Key: strings.TrimSpace(f.TotalLabel)}
	if sum.Key == "" {
		sum.Key = "Summe"
	}
	for _, rec := range records {
		sum.IncomeCents += rec.IncomeCents
		sum.ExpenseCents += rec.ExpenseCents
		sum.Count += rec.Count
	}
	return project(f, sum)
}

// value renders one field of a record. Validate closes the field set, so the default
// is unreachable from a validated format.
func value(f Field, rec Record) string {
	switch f {
	case FieldKey:
		return rec.Key
	case FieldIncome:
		return money(rec.IncomeCents)
	case FieldExpense:
		return money(rec.ExpenseCents)
	case FieldNet:
		return money(rec.NetCents())
	case FieldCount:
		return strconv.Itoa(rec.Count)
	default:
		return ""
	}
}

// money renders minor units as a decimal string ("125000" -> "1250.00"), the shape the
// rest of the repository uses for money.
func money(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	return sign + strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
