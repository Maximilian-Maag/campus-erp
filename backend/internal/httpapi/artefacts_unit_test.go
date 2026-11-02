package httpapi

import "testing"

// TestArtefactKey covers the allowlist and the name rules: the route must accept a bare
// name inside a known kind and nothing else.
func TestArtefactKey(t *testing.T) {
	cases := []struct {
		kind, name string
		want       string
		wantErr    bool
	}{
		{"dunning", "7.pdf", "dunning/7.pdf", false},
		{"reports", "daily-close.csv", "reports/daily-close.csv", false},
		{"ledger", "postings.csv", "ledger/postings.csv", false},
		{"sepa", "run-1.xml", "sepa/run-1.xml", false},
		{"dunning", " 7.pdf ", "dunning/7.pdf", false},
		{"keys", "7.pdf", "", true},
		{"", "7.pdf", "", true},
		{"dunning", "", "", true},
		{"dunning", ".", "", true},
		{"dunning", "..", "", true},
		{"dunning", "a/b", "", true},
	}
	for _, c := range cases {
		got, err := artefactKey(c.kind, c.name)
		if c.wantErr {
			if err == nil {
				t.Errorf("artefactKey(%q, %q) = %q, want an error", c.kind, c.name, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("artefactKey(%q, %q) = %q, %v; want %q", c.kind, c.name, got, err, c.want)
		}
	}
}

// TestArtefactContentType: the kind pins the type, the extension overrides it for a
// report that is not a PDF, and anything unrecognised falls back to octet-stream.
func TestArtefactContentType(t *testing.T) {
	cases := map[string]string{
		"dunning/7.pdf":       "application/pdf",
		"reports/daily.csv":   "text/csv",
		"reports/daily.pdf":   "application/pdf",
		"ledger/postings.csv": "text/csv",
		"ledger/postings.dat": "text/csv",
		"sepa/run-1.xml":      "application/xml",
		"sepa/run-1.bin":      "application/xml",
		"unknown/thing.bin":   "application/octet-stream",
		"unknown/thing.xml":   "application/xml",
		"unknown/THING.CSV":   "text/csv",
	}
	for key, want := range cases {
		if got := artefactContentType(key); got != want {
			t.Errorf("artefactContentType(%q) = %q, want %q", key, got, want)
		}
	}
}
