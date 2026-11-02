package httpapi_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtefactRoute serves stored artefacts of every kind over HTTP.
func TestArtefactRoute(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := artefactsDir(t)
	write := func(rel string, content string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("reports/daily-close.csv", "Betrag\n125,00\n")
	write("ledger/postings.csv", "date,amount\n2026-11-02,125\n")
	write("sepa/run-1.xml", "<Document/>")
	write("dunning/9.pdf", "%PDF-1.7 notice")

	cases := []struct {
		url, contentType, body string
	}{
		{"/api/artefacts/reports/daily-close.csv", "text/csv", "Betrag"},
		{"/api/artefacts/ledger/postings.csv", "text/csv", "date,amount"},
		{"/api/artefacts/sepa/run-1.xml", "application/xml", "<Document/>"},
		{"/api/artefacts/dunning/9.pdf", "application/pdf", "%PDF-1.7"},
	}
	for _, c := range cases {
		resp := doJSON(t, http.MethodGet, srv.URL+c.url, "carol", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d, want 200", c.url, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != c.contentType {
			t.Fatalf("%s content type = %q, want %q", c.url, ct, c.contentType)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read %s: %v", c.url, err)
		}
		if !strings.Contains(string(body), c.body) {
			t.Fatalf("%s body = %q, want it to contain %q", c.url, body, c.body)
		}
	}

	// A kind the route does not serve stays out of reach, even if such a key exists.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/artefacts/keys/master", "carol", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d, want 400", resp.StatusCode)
	}
	// A known kind, nothing stored under that name.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/artefacts/reports/absent.csv", "carol", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("absent artefact = %d, want 404", resp.StatusCode)
	}
}
