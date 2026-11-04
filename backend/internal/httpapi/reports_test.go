package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

// TestListReportsRoute: the API offers the reports the catalogue holds, from the same list the
// worker renders from, so a client cannot be told about a report that does not render.
func TestListReportsRoute(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/reports", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
	var defs []struct {
		ID      string
		Title   string
		Columns []struct{ Key, Title, Align string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&defs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("reports = %d, want 3", len(defs))
	}
	if defs[0].ID != "kasse-daily-close" || defs[1].ID != "budget-position-overview" || defs[2].ID != "klr-cost-center-overview" {
		t.Fatalf("ids = %q, %q, %q", defs[0].ID, defs[1].ID, defs[2].ID)
	}
	// The definitions carry their columns, so a client can offer the report without guessing.
	if defs[1].Title != "Mittelübersicht" || len(defs[1].Columns) != 8 {
		t.Fatalf("budget overview = %+v", defs[1])
	}
	if defs[1].Columns[7].Key != "available" || defs[1].Columns[7].Align != "right" {
		t.Fatalf("last column = %+v", defs[1].Columns[7])
	}
	if defs[2].Title != "Kostenstellenübersicht" || len(defs[2].Columns) != 5 {
		t.Fatalf("cost-centre overview = %+v", defs[2])
	}
	if defs[2].Columns[0].Key != "number" || defs[2].Columns[4].Key != "balance" || defs[2].Columns[4].Align != "right" {
		t.Fatalf("cost-centre columns = %+v", defs[2].Columns)
	}
}

// TestListReportsWithoutACatalogue: an API wired without one says so rather than panicking or
// answering with an empty list that looks like "there are no reports".
func TestListReportsWithoutACatalogue(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{}).Routes())
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/reports", "alice", "")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
