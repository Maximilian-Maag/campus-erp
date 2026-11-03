package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

// problemCode decodes the machine code out of an error envelope.
func problemCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var p httpapi.Problem
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("body is not a Problem: %v", err)
	}
	return p.Code
}

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
	write("statistics/bw-20261101-20261130.csv", "Zweck;Einnahmen\nMensa;10.00\n")

	cases := []struct {
		url, contentType, body string
	}{
		{"/api/artefacts/reports/daily-close.csv", "text/csv", "Betrag"},
		{"/api/artefacts/ledger/postings.csv", "text/csv", "date,amount"},
		{"/api/artefacts/sepa/run-1.xml", "application/xml", "<Document/>"},
		{"/api/artefacts/dunning/9.pdf", "application/pdf", "%PDF-1.7"},
		{"/api/artefacts/statistics/bw-20261101-20261130.csv", "text/csv", "Zweck;Einnahmen"},
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
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/artefacts/keys/master", "carol", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d, want 400", resp.StatusCode)
	}
	if code := problemCode(t, resp); code != "invalid_artefact" {
		t.Fatalf("unknown kind code = %q, want invalid_artefact", code)
	}
	// A known kind, nothing stored under that name.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/artefacts/reports/absent.csv", "carol", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("absent artefact = %d, want 404", resp.StatusCode)
	}
	if code := problemCode(t, resp); code != "artefact_not_found" {
		t.Fatalf("absent artefact code = %q, want artefact_not_found", code)
	}
}
