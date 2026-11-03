package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
)

func TestHTTPLookupFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	// Seed the standard catalogues.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/lookups/seed", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed = %d", resp.StatusCode)
	}
	var seeded struct{ Seeded int }
	if err := json.NewDecoder(resp.Body).Decode(&seeded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if seeded.Seeded < 30 {
		t.Fatalf("seeded = %d", seeded.Seeded)
	}

	// List a namespace.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/lookups/vat_category", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d", resp.StatusCode)
	}
	var values []struct {
		Namespace string
		Code      string
		ShortText string
	}
	if err := json.NewDecoder(resp.Body).Decode(&values); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(values) != len(core.VATCategories()) {
		t.Fatalf("values = %d, want core's %d", len(values), len(core.VATCategories()))
	}

	// Get one.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/lookups/vat_category/S", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("get = %d", resp.StatusCode)
	}

	// Bulk upsert into a fresh namespace.
	ns := "test_" + uniqueSuffix(t)
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/lookups/"+ns, "alice",
		`{"values":[{"code":"A","short_text":"alpha","sort_order":1,"active":true}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert = %d", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/lookups/"+ns, "alice", "")
	var one []struct{ Code string }
	_ = json.NewDecoder(resp.Body).Decode(&one)
	if len(one) != 1 || one[0].Code != "A" {
		t.Fatalf("upserted = %+v", one)
	}
}

func TestHTTPLookupErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// Unknown value -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/lookups/nope/nope", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", resp.StatusCode)
	}
	// Malformed body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/lookups/ns", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Validation failure (no short text) -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/lookups/ns", "a",
		`{"values":[{"code":"A","short_text":""}]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation = %d, want 422", resp.StatusCode)
	}
	// Empty namespace list is fine (200, empty).
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/lookups/empty_namespace", "a", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("empty = %d, want 200", resp.StatusCode)
	}
}
