package report_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

func def() report.Definition {
	return report.Definition{ID: "r1", Title: "Bericht", Columns: []report.Column{
		{Key: "a", Title: "A", Align: report.Left},
		{Key: "b", Title: "B", Align: report.Right},
	}}
}

func TestDefinitionValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*report.Definition)
		want   error
	}{
		{"no id", func(d *report.Definition) { d.ID = "  " }, report.ErrIDRequired},
		{"no title", func(d *report.Definition) { d.Title = "" }, report.ErrTitleRequired},
		{"no columns", func(d *report.Definition) { d.Columns = nil }, report.ErrColumnsRequired},
		{"key", func(d *report.Definition) { d.Columns[0].Key = " " }, report.ErrColumnIncomplete},
		{"title", func(d *report.Definition) { d.Columns[0].Title = "" }, report.ErrColumnIncomplete},
		{"duplicate", func(d *report.Definition) { d.Columns[1].Key = "a" }, report.ErrDuplicateColumn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := def()
			c.mutate(&d)
			if err := d.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("Validate = %v, want %v", err, c.want)
			}
		})
	}
	if err := def().Validate(); err != nil {
		t.Fatalf("a complete definition must validate: %v", err)
	}
}

func TestReportValidateRowWidth(t *testing.T) {
	r := report.Report{Definition: def(), Rows: [][]string{{"x"}}}
	if err := r.Validate(); !errors.Is(err, report.ErrRowWidth) {
		t.Fatalf("Validate = %v, want ErrRowWidth", err)
	}
	if _, err := r.MarshalCSV(); !errors.Is(err, report.ErrRowWidth) {
		t.Fatalf("MarshalCSV = %v, want ErrRowWidth", err)
	}
	if _, err := r.MarshalHTML(); !errors.Is(err, report.ErrRowWidth) {
		t.Fatalf("MarshalHTML = %v, want ErrRowWidth", err)
	}
}

func TestMarshalCSV(t *testing.T) {
	r := report.Report{Definition: def(), Rows: [][]string{{"1", "2"}, {"komma, drin", `"zitat"`}}}
	got, err := r.MarshalCSV()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "A,B\n") {
		t.Fatalf("csv = %q", got)
	}
	if !strings.Contains(got, `"komma, drin","""zitat"""`) {
		t.Fatalf("csv quoting missing: %q", got)
	}
}

func TestMarshalHTML(t *testing.T) {
	r := report.Report{Definition: def(), Rows: [][]string{{"<x>", "1"}}, GeneratedAt: time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)}
	got, err := r.MarshalHTML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Bericht</title>", "&lt;x&gt;", `class="right"`, "2026-11-02 09:30"} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q:\n%s", want, got)
		}
	}

	// an unset timestamp is simply left out
	r.GeneratedAt = time.Time{}
	got, _ = r.MarshalHTML()
	if strings.Contains(got, "Stand: 0001-01-01") {
		t.Fatalf("zero timestamp must not render: %s", got)
	}
}

func TestAlignmentValid(t *testing.T) {
	for _, a := range []report.Alignment{report.Left, report.Right} {
		if !a.Valid() {
			t.Errorf("%q must be valid", a)
		}
	}
	for _, a := range []report.Alignment{"", "center"} {
		if a.Valid() {
			t.Errorf("%q must be invalid", a)
		}
	}
}
