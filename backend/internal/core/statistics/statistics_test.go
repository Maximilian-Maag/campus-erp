package statistics

import (
	"errors"
	"reflect"
	"testing"
)

func TestFormatValidate(t *testing.T) {
	valid := Format{
		State:     "x",
		Name:      "X",
		Delimiter: ';',
		Columns:   []Column{{Field: FieldKey, Title: "Zweck"}},
	}
	cases := []struct {
		name    string
		mutate  func(Format) Format
		wantErr error
	}{
		{"valid", func(f Format) Format { return f }, nil},
		{"empty state", func(f Format) Format { f.State = "  "; return f }, ErrStateRequired},
		{"empty name", func(f Format) Format { f.Name = ""; return f }, ErrNameRequired},
		{"delimiter zero", func(f Format) Format { f.Delimiter = 0; return f }, ErrDelimiterInvalid},
		{"delimiter quote", func(f Format) Format { f.Delimiter = '"'; return f }, ErrDelimiterInvalid},
		{"no columns", func(f Format) Format { f.Columns = nil; return f }, ErrColumnsRequired},
		{"unknown field", func(f Format) Format {
			f.Columns = []Column{{Field: "nope", Title: "T"}}
			return f
		}, ErrColumnIncomplete},
		{"empty title", func(f Format) Format {
			f.Columns = []Column{{Field: FieldKey, Title: " "}}
			return f
		}, ErrColumnIncomplete},
		{"duplicate field", func(f Format) Format {
			f.Columns = []Column{{Field: FieldKey, Title: "A"}, {Field: FieldKey, Title: "B"}}
			return f
		}, ErrDuplicateField},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.mutate(valid).Validate()
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Validate = %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestRegistryNew(t *testing.T) {
	a := Format{State: "aa", Name: "A", Delimiter: ',', Columns: []Column{{Field: FieldKey, Title: "K"}}}
	b := Format{State: "bb", Name: "B", Delimiter: ',', Columns: []Column{{Field: FieldCount, Title: "N"}}}

	r, err := New([]Format{b, a}) // deliberately out of order
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := r.States(); !reflect.DeepEqual(got, []string{"aa", "bb"}) {
		t.Fatalf("States = %v, want [aa bb] (ordered)", got)
	}
	if got := r.All(); len(got) != 2 || got[0].State != "aa" {
		t.Fatalf("All = %v, want ordered with aa first", got)
	}
	if f, ok := r.Get("bb"); !ok || f.State != "bb" {
		t.Fatalf("Get(bb) = %v, %v", f, ok)
	}
	if _, ok := r.Get("zz"); ok {
		t.Fatal("Get(zz) = ok, want not found")
	}

	if _, err := New([]Format{a, a}); err == nil {
		t.Fatal("New with a duplicate state = nil, want an error")
	}
	if _, err := New([]Format{{State: "", Name: "x", Delimiter: ',', Columns: []Column{{Field: FieldKey, Title: "K"}}}}); !errors.Is(err, ErrStateRequired) {
		t.Fatalf("New with an invalid format = %v, want ErrStateRequired", err)
	}
}

func TestStandardRegistry(t *testing.T) {
	r := Standard()
	// Every seeded layout must be a usable format, reachable by its state.
	for _, f := range r.All() {
		if err := f.Validate(); err != nil {
			t.Fatalf("standard format %q invalid: %v", f.State, err)
		}
		if got, ok := r.Get(f.State); !ok || got.State != f.State {
			t.Fatalf("Get(%q) = %v, %v", f.State, got, ok)
		}
	}
	if got := r.States(); !reflect.DeepEqual(got, []string{"bw", "by", "default"}) {
		t.Fatalf("States = %v, want [bw by default]", got)
	}
}

func TestValue(t *testing.T) {
	rec := Record{Key: "Mensa", IncomeCents: 125000, ExpenseCents: 25000, Count: 3}
	cases := map[Field]string{
		FieldKey:     "Mensa",
		FieldIncome:  "1250.00",
		FieldExpense: "250.00",
		FieldNet:     "1000.00",
		FieldCount:   "3",
	}
	for f, want := range cases {
		if got := value(f, rec); got != want {
			t.Errorf("value(%q) = %q, want %q", f, got, want)
		}
	}
	if got := value(Field("unknown"), rec); got != "" {
		t.Errorf("value(unknown) = %q, want \"\"", got)
	}
}

func TestMoney(t *testing.T) {
	cases := map[int64]string{
		0:       "0.00",
		5:       "0.05",
		99:      "0.99",
		100:     "1.00",
		125000:  "1250.00",
		125005:  "1250.05",
		-125005: "-1250.05",
	}
	for cents, want := range cases {
		if got := money(cents); got != want {
			t.Errorf("money(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestGroup(t *testing.T) {
	records, err := Group([]Entry{
		{Key: "Mensa", Kind: Income, AmountCents: 100000},
		{Key: "Semesterbeitrag", Kind: Income, AmountCents: 30000},
		{Key: "Mensa", Kind: Expense, AmountCents: 25000},
		{Key: " Mensa ", Kind: Income, AmountCents: 25000}, // trimmed onto the same key
	})
	if err != nil {
		t.Fatalf("Group: %v", err)
	}
	want := []Record{
		{Key: "Mensa", IncomeCents: 125000, ExpenseCents: 25000, Count: 3},
		{Key: "Semesterbeitrag", IncomeCents: 30000, ExpenseCents: 0, Count: 1},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("Group = %+v, want %+v", records, want)
	}
	if got := records[0].NetCents(); got != 100000 {
		t.Fatalf("NetCents = %d, want 100000", got)
	}

	if _, err := Group([]Entry{{Key: "  ", Kind: Income}}); !errors.Is(err, ErrEntryKeyRequired) {
		t.Fatalf("Group with no key = %v, want ErrEntryKeyRequired", err)
	}
	if _, err := Group([]Entry{{Key: "x", Kind: "sideways"}}); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("Group with an unknown kind = %v, want ErrUnknownKind", err)
	}
	if got, err := Group(nil); err != nil || len(got) != 0 {
		t.Fatalf("Group(nil) = %v, %v; want empty, nil", got, err)
	}
}

func TestRenderDefault(t *testing.T) {
	r := Standard()
	f, _ := r.Get("default")
	got, err := Render(f, []Record{
		{Key: "Mensa", IncomeCents: 125000, ExpenseCents: 25000, Count: 3},
		{Key: "Semesterbeitrag", IncomeCents: 30000, Count: 1},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "Zweck;Einnahmen;Ausgaben;Saldo;Buchungen\n" +
		"Mensa;1250.00;250.00;1000.00;3\n" +
		"Semesterbeitrag;300.00;0.00;300.00;1\n" +
		"Summe;1550.00;250.00;1300.00;4\n"
	if got != want {
		t.Fatalf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderCaptionAndTabDelimiter(t *testing.T) {
	r := Standard()

	bw, _ := r.Get("bw")
	got, err := Render(bw, []Record{{Key: "Mensa", IncomeCents: 125000, ExpenseCents: 25000, Count: 3}})
	if err != nil {
		t.Fatalf("Render(bw): %v", err)
	}
	want := "Finanzstatistik\n" +
		"Zweck;Anzahl;Einnahmen (EUR);Ausgaben (EUR);Ueberschuss (EUR)\n" +
		"Mensa;3;1250.00;250.00;1000.00\n" +
		"Gesamt;3;1250.00;250.00;1000.00\n"
	if got != want {
		t.Fatalf("Render(bw) =\n%q\nwant\n%q", got, want)
	}

	by, _ := r.Get("by")
	got, err = Render(by, []Record{{Key: "Mensa", IncomeCents: 125000}})
	if err != nil {
		t.Fatalf("Render(by): %v", err)
	}
	want = "Zweck\tEinnahmen\tAusgaben\n" +
		"Mensa\t1250.00\t0.00\n"
	if got != want {
		t.Fatalf("Render(by) =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderTotalsLabelDefaultAndTrim(t *testing.T) {
	f := Format{
		State:     "x",
		Name:      "X",
		Delimiter: ',',
		Title:     "  Umsatz  ",
		Columns:   []Column{{Field: FieldKey, Title: "Zweck"}, {Field: FieldCount, Title: "Anzahl"}},
		Totals:    true,
	}
	got, err := Render(f, []Record{{Key: "Mensa", Count: 2}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "Umsatz\n" +
		"Zweck,Anzahl\n" +
		"Mensa,2\n" +
		"Summe,2\n" // empty TotalLabel falls back to "Summe"
	if got != want {
		t.Fatalf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderRefusesInvalidFormat(t *testing.T) {
	f := Format{State: "x", Name: "X", Delimiter: ';'} // a state and a name, but no columns
	if _, err := Render(f, nil); !errors.Is(err, ErrColumnsRequired) {
		t.Fatalf("Render with no columns = %v, want ErrColumnsRequired", err)
	}
}
