// Package report defines what a report is — a titled, column-defined table — and
// renders it to a printable document or CSV. The definitions and their data belong to
// the domain that owns them; this package owns the shape and the output formats
// (docs/25, ADR-0010).
package report

import (
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// Errors of a report.
var (
	ErrIDRequired       = errors.New("report: an id is required")
	ErrTitleRequired    = errors.New("report: a title is required")
	ErrColumnsRequired  = errors.New("report: at least one column is required")
	ErrColumnIncomplete = errors.New("report: every column needs a key and a title")
	ErrDuplicateColumn  = errors.New("report: column keys must be unique")
	ErrRowWidth         = errors.New("report: a row does not match the column count")
)

// Alignment is how a column's values sit.
type Alignment string

// The supported alignments.
const (
	Left  Alignment = "left"
	Right Alignment = "right"
)

// Valid reports whether a is a known alignment.
func (a Alignment) Valid() bool { return a == Left || a == Right }

// Column is one column: a stable key, a heading and an alignment.
type Column struct {
	Key   string
	Title string
	Align Alignment
}

// Definition is a report's shape.
type Definition struct {
	ID      string
	Title   string
	Columns []Column
}

// Validate checks the definition is usable.
func (d Definition) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return ErrIDRequired
	}
	if strings.TrimSpace(d.Title) == "" {
		return ErrTitleRequired
	}
	if len(d.Columns) == 0 {
		return ErrColumnsRequired
	}
	seen := make(map[string]bool, len(d.Columns))
	for _, c := range d.Columns {
		if strings.TrimSpace(c.Key) == "" || strings.TrimSpace(c.Title) == "" {
			return ErrColumnIncomplete
		}
		if seen[c.Key] {
			return fmt.Errorf("%w: %q", ErrDuplicateColumn, c.Key)
		}
		seen[c.Key] = true
	}
	return nil
}

// Report is a definition with the rows rendered from it.
type Report struct {
	Definition
	Rows        [][]string
	GeneratedAt time.Time
}

// Validate checks the definition and that every row matches the columns.
func (r Report) Validate() error {
	if err := r.Definition.Validate(); err != nil {
		return err
	}
	for i, row := range r.Rows {
		if len(row) != len(r.Columns) {
			return fmt.Errorf("%w: row %d has %d of %d", ErrRowWidth, i+1, len(row), len(r.Columns))
		}
	}
	return nil
}

// MarshalCSV renders the report as CSV, one row per row, headed by the column titles.
func (r Report) MarshalCSV() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	titles := make([]string, 0, len(r.Columns))
	for _, c := range r.Columns {
		titles = append(titles, c.Title)
	}
	if err := w.Write(titles); err != nil {
		return "", fmt.Errorf("report: header: %w", err)
	}
	for _, row := range r.Rows {
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("report: row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("report: %w", err)
	}
	return b.String(), nil
}

var htmlTemplate = template.Must(template.New("report").Parse(
	`<!doctype html>
<html lang="de">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body>
<h1>{{.Title}}</h1>
<table>
<thead><tr>{{range .Columns}}<th class="{{.Align}}">{{.Title}}</th>{{end}}</tr></thead>
<tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody>
</table>
<p class="generated">Stand: {{.GeneratedAt}}</p>
</body>
</html>
`))

// MarshalHTML renders the report as a printable document for the document service.
func (r Report) MarshalHTML() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	data := struct {
		Title       string
		Columns     []Column
		Rows        [][]string
		GeneratedAt string
	}{
		Title:   strings.TrimSpace(r.Title),
		Columns: r.Columns,
		Rows:    r.Rows,
	}
	if !r.GeneratedAt.IsZero() {
		data.GeneratedAt = r.GeneratedAt.Format("2006-01-02 15:04")
	}
	if err := htmlTemplate.Execute(&b, data); err != nil {
		return "", fmt.Errorf("report: html: %w", err)
	}
	return b.String(), nil
}
