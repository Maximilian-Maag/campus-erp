package budgetimport_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
)

// The layout is data, so the sample file is the layout written down: it must parse with nothing but
// the standard layout, and it must stay readable next to the Layout value it belongs to.
func TestParseReadsTheSampleFile(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "budget-import-v1.csv"))
	if err != nil {
		t.Fatalf("read the sample: %v", err)
	}
	rows, err := budgetimport.Parse(budgetimport.StandardLayout, string(content))
	if err != nil {
		t.Fatalf("parse the sample: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	want := budgetimport.Row{
		Line: 2, SourceCode: "LHH", SourceName: "Landesmittel", SourceChapter: "05",
		CenterCode: "BW-1", CenterName: "Fakultaet 1", Year: 2026, Chapter: "05",
		Title: "1234", Description: "Beschaffung", AllocatedCents: 1000000,
	}
	if rows[0] != want {
		t.Fatalf("row 0 = %+v, want %+v", rows[0], want)
	}
	// A row that names no source still carries its centre and its position.
	if rows[3].SourceCode != "" || rows[3].CenterCode != "BW-1" || rows[3].AllocatedCents != 100000 {
		t.Fatalf("row 3 = %+v, want no source, BW-1 and 1000.00", rows[3])
	}
	// The chapter-less third-party row keeps its empty chapter.
	if rows[2].SourceChapter != "" || rows[2].SourceCode != "DFG" {
		t.Fatalf("row 2 = %+v, want DFG without a chapter", rows[2])
	}
}

func TestLayoutByID(t *testing.T) {
	got, err := budgetimport.LayoutByID("")
	if err != nil || got.ID != budgetimport.StandardLayout.ID {
		t.Fatalf("empty id = %+v, %v, want the standard layout", got, err)
	}
	got, err = budgetimport.LayoutByID("budget-import/v1")
	if err != nil || got.Delimiter != ";" {
		t.Fatalf("by id = %+v, %v, want the standard layout", got, err)
	}
	if _, err := budgetimport.LayoutByID("vendor-import/v9"); !errors.Is(err, budgetimport.ErrUnknownLayout) {
		t.Fatalf("unknown id err = %v, want ErrUnknownLayout", err)
	}
	if len(budgetimport.Layouts()) != 1 || budgetimport.Layouts()[0].ID != "budget-import/v1" {
		t.Fatalf("layouts = %+v, want the standard one alone", budgetimport.Layouts())
	}
}

func TestParseRefusals(t *testing.T) {
	body := "A;Mittel;;B;Stelle;2026;05;1234;Beschaffung;100,00\n"
	cases := []struct {
		name    string
		layout  budgetimport.Layout
		content string
		want    error
	}{
		{"nothing at all", budgetimport.StandardLayout, "", budgetimport.ErrEmpty},
		{"only a header", budgetimport.StandardLayout, "a;b;c;d;e;f;g;h;i;j\n", budgetimport.ErrEmpty},
		{"only blank lines", budgetimport.StandardLayout, "\n \n\t\n", budgetimport.ErrEmpty},
		{"a row without enough columns", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;2026;05\n", budgetimport.ErrBadRow},
		{"a year that is not a year", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;zwanzig;05;9;x;1,00\n", budgetimport.ErrBadRow},
		{"a year outside any budget", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;1899;05;9;x;1,00\n", budgetimport.ErrBadRow},
		{"a missing Kapitel", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;2026;;9;x;1,00\n", budgetimport.ErrBadRow},
		{"a missing Titel", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;2026;05;;x;1,00\n", budgetimport.ErrBadRow},
		{"a negative Ansatz", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;2026;05;9;x;-1,00\n", budgetimport.ErrBadRow},
		{"an Ansatz with three decimals", budgetimport.StandardLayout, body + "A;Mittel;;B;Stelle;2026;05;9;x;1,234\n", budgetimport.ErrBadRow},
		{"a code without a name", budgetimport.StandardLayout, body + "A;;;B;Stelle;2026;05;9;x;1,00\n", budgetimport.ErrBadRow},
		{"a centre code without a name", budgetimport.StandardLayout, body + "A;Mittel;;B;;2026;05;9;x;1,00\n", budgetimport.ErrBadRow},
		{
			"the same position twice",
			budgetimport.StandardLayout,
			"h;h;h;h;h;h;h;h;h;h\n" + body + "A;Mittel;;B;Stelle;2026;05;1234;Beschaffung;200,00\n",
			budgetimport.ErrBadRow,
		},
		{
			"a layout with no delimiter",
			budgetimport.Layout{ID: "x", Columns: budgetimport.StandardLayout.Columns},
			body, budgetimport.ErrUnknownLayout,
		},
		{
			"a layout with a two-character delimiter",
			budgetimport.Layout{ID: "x", Delimiter: ";;", Columns: budgetimport.StandardLayout.Columns},
			body, budgetimport.ErrUnknownLayout,
		},
		{
			"a layout that does not name the year",
			budgetimport.Layout{ID: "x", Delimiter: ";", Columns: map[budgetimport.Field]int{
				budgetimport.FieldChapter: 0, budgetimport.FieldTitle: 1, budgetimport.FieldAllocated: 2,
			}},
			"05;1234;100,00\n", budgetimport.ErrBadRow,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, err := budgetimport.Parse(c.layout, c.content)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if rows != nil {
				t.Fatalf("rows = %+v, want none: a refused file returns nothing at all", rows)
			}
		})
	}
}

// A refusal names the line, because a budget file is hand-edited.
func TestBadRowNamesItsLine(t *testing.T) {
	_, err := budgetimport.Parse(budgetimport.StandardLayout,
		"h;h;h;h;h;h;h;h;h;h\nA;Mittel;;B;Stelle;2026;05;1234;x;1,00\nA;Mittel;;B;Stelle;2026;05;9999;x;nope\n")
	if !errors.Is(err, budgetimport.ErrBadRow) {
		t.Fatalf("err = %v, want a bad row", err)
	}
	if got := err.Error(); !strings.Contains(got, "row 3") {
		t.Fatalf("err = %q, want it to name row 3", got)
	}
}

// A layout without a header reads its first line as data.
func TestParseWithoutAHeader(t *testing.T) {
	layout := budgetimport.StandardLayout
	layout.Header = false
	rows, err := budgetimport.Parse(layout, "A;Mittel;;B;Stelle;2026;05;1234;x;1,00\n")
	if err != nil || len(rows) != 1 || rows[0].Title != "1234" {
		t.Fatalf("rows = %+v, err = %v, want one row including the first line", rows, err)
	}
}

// The Ansatz may be empty — a title can be opened without an Ansatz yet — and the year's ends are
// the ends, not one past them.
func TestParseAcceptsTheEdges(t *testing.T) {
	// An empty Ansatz is zero, not a refusal.
	rows, err := budgetimport.Parse(budgetimport.StandardLayout,
		"h;h;h;h;h;h;h;h;h;h\nA;Mittel;;B;Stelle;2026;05;9;noch offen;\n")
	if err != nil || len(rows) != 1 || rows[0].AllocatedCents != 0 {
		t.Fatalf("rows = %+v, err = %v, want one row with an Ansatz of zero", rows, err)
	}
	// A year at either end of the range is still a Haushaltsjahr.
	for _, year := range []string{"2000", "2200"} {
		rows, err := budgetimport.Parse(budgetimport.StandardLayout,
			"h;h;h;h;h;h;h;h;h;h\nA;Mittel;;B;Stelle;"+year+";05;9;x;1,00\n")
		if err != nil || len(rows) != 1 || fmt.Sprint(rows[0].Year) != year {
			t.Fatalf("year %s: rows = %+v, err = %v", year, rows, err)
		}
	}
	for _, year := range []string{"1999", "2201"} {
		if _, err := budgetimport.Parse(budgetimport.StandardLayout,
			"h;h;h;h;h;h;h;h;h;h\nA;Mittel;;B;Stelle;"+year+";05;9;x;1,00\n"); !errors.Is(err, budgetimport.ErrBadRow) {
			t.Fatalf("year %s: err = %v, want a bad row", year, err)
		}
	}
}

// A short row names the column the file is missing, so a hand-edited file can be fixed.
func TestShortRowNamesTheMissingColumn(t *testing.T) {
	_, err := budgetimport.Parse(budgetimport.StandardLayout, "h;h;h;h;h;h;h;h;h;h\nA;Mittel;;B;Stelle;2026\n")
	if !errors.Is(err, budgetimport.ErrBadRow) {
		t.Fatalf("err = %v, want a bad row", err)
	}
	if got := err.Error(); !strings.Contains(got, "column 7") {
		t.Fatalf("err = %q, want it to name column 7", got)
	}
}

func TestParseEuros(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"1,00", 100, false},
		{"10.000,00", 1000000, false},
		{"2.500,5", 250050, false},
		{"1234.56", 123456, false},
		{"1", 100, false},
		// A single dot with one digit after it is a machine's decimal mark too.
		{"1.5", 150, false},
		{"0.5", 50, false},
		// A dot with three digits after it is a thousands separator, not a decimal mark.
		{"1.234", 123400, false},
		{"1.234.567", 123456700, false},
		{"  7,77  ", 777, false},
		{"-1,00", 0, true},
		{"1,234", 0, true},
		{"zwei", 0, true},
	}
	for _, c := range cases {
		got, err := budgetimport.ParseEuros(c.in)
		if c.bad {
			if err == nil {
				t.Fatalf("ParseEuros(%q) = %d, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseEuros(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseEuros(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
