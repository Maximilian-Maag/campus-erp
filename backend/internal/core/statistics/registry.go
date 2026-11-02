package statistics

import (
	"fmt"
	"slices"
	"strings"
)

// Registry is the set of per-state formats. It is the seam a state's layout lives
// behind: the pipeline asks the registry for a state and renders whatever it returns,
// so a state is added or changed without any pipeline change.
type Registry struct {
	byState map[string]Format
	list    []Format
}

// New validates and stores a set of formats.
func New(formats []Format) (*Registry, error) {
	r := &Registry{byState: make(map[string]Format, len(formats)), list: append([]Format(nil), formats...)}
	for _, f := range formats {
		if err := f.Validate(); err != nil {
			return nil, err
		}
		if _, dup := r.byState[f.State]; dup {
			return nil, fmt.Errorf("statistics: duplicate state %q", f.State)
		}
		r.byState[f.State] = f
	}
	slices.SortFunc(r.list, func(a, b Format) int { return strings.Compare(a.State, b.State) })
	return r, nil
}

// Standard returns the registry seeded with the formats known today.
//
// These layouts are PROJECT-DEFINED PLACEHOLDERS: the offices' real per-state layouts
// are not published (docs/open-questions Q8, docs/27). They exist so the pipeline is
// exercised end to end and so the shape of a layout is concrete. Replacing one is
// editing its entry below; replacing all of them touches no other file.
func Standard() *Registry {
	r, err := New([]Format{
		{
			State:     "default",
			Name:      "Neutral (Vorlage)",
			Delimiter: ';',
			Columns: []Column{
				{Field: FieldKey, Title: "Zweck"},
				{Field: FieldIncome, Title: "Einnahmen"},
				{Field: FieldExpense, Title: "Ausgaben"},
				{Field: FieldNet, Title: "Saldo"},
				{Field: FieldCount, Title: "Buchungen"},
			},
			Totals:     true,
			TotalLabel: "Summe",
		},
		{
			State:     "bw",
			Name:      "Baden-Wuerttemberg (Vorlage)",
			Delimiter: ';',
			Title:     "Finanzstatistik",
			Columns: []Column{
				{Field: FieldKey, Title: "Zweck"},
				{Field: FieldCount, Title: "Anzahl"},
				{Field: FieldIncome, Title: "Einnahmen (EUR)"},
				{Field: FieldExpense, Title: "Ausgaben (EUR)"},
				{Field: FieldNet, Title: "Ueberschuss (EUR)"},
			},
			Totals:     true,
			TotalLabel: "Gesamt",
		},
		{
			State:     "by",
			Name:      "Bayern (Vorlage)",
			Delimiter: '\t',
			Columns: []Column{
				{Field: FieldKey, Title: "Zweck"},
				{Field: FieldIncome, Title: "Einnahmen"},
				{Field: FieldExpense, Title: "Ausgaben"},
			},
		},
	})
	if err != nil {
		// The seed above is a compile-time constant; a violation is a bug.
		panic("statistics: standard registry invalid: " + err.Error())
	}
	return r
}

// Get returns one format by state.
func (r *Registry) Get(state string) (Format, bool) {
	f, ok := r.byState[state]
	return f, ok
}

// States returns the registered states, ordered.
func (r *Registry) States() []string {
	out := make([]string, 0, len(r.list))
	for _, f := range r.list {
		out = append(out, f.State)
	}
	return out
}

// All returns the formats, ordered by state.
func (r *Registry) All() []Format { return r.list }
