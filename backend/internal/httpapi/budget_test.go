package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type budgetPositionJSON struct {
	Position struct {
		ID             int64
		Year           int
		Chapter        string
		Title          string
		AllocatedCents int64
	}
	Balance struct {
		PositionID     int64
		AllocatedCents int64
		CommittedCents int64
		ActualCents    int64
	}
}

func TestHTTPBudgetFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "alice",
		`{"year":2026,"chapter":"05","title":"T-`+uniqueSuffix(t)+`","description":"Beschaffung","allocated_cents":100000}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create position = %d", resp.StatusCode)
	}
	var pos struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&pos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pos.ID == 0 {
		t.Fatal("no id")
	}

	// Commit 30000.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(pos.ID)+"/postings", "alice",
		`{"kind":"commitment","amount_cents":30000,"reference":"PO-1"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("commitment = %d", resp.StatusCode)
	}

	// Position view carries the balance.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/"+itoa(pos.ID), "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get position = %d", resp.StatusCode)
	}
	var view budgetPositionJSON
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if view.Balance.AllocatedCents != 100000 || view.Balance.CommittedCents != 30000 {
		t.Fatalf("balance = %+v", view.Balance)
	}

	// Over-spend -> 409.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(pos.ID)+"/postings", "alice",
		`{"kind":"commitment","amount_cents":80000,"reference":"PO-2"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("overspend = %d, want 409", resp.StatusCode)
	}

	// Balance endpoint.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/"+itoa(pos.ID)+"/balance", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("balance = %d", resp.StatusCode)
	}
}

func TestHTTPBudgetErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Validation failure (no chapter) -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "a",
		`{"year":2026,"chapter":"","title":"X","allocated_cents":1}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation = %d, want 422", resp.StatusCode)
	}
	// Duplicate -> 409.
	title := "DUP-" + uniqueSuffix(t)
	body := `{"year":2026,"chapter":"05","title":"` + title + `","allocated_cents":1000}`
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "a", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "a", body); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", resp.StatusCode)
	}
	// Unknown position -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/9223372036854775807", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", resp.StatusCode)
	}
	// Bad id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/abc/balance", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	// Malformed posting body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/1/postings", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad posting body = %d, want 400", resp.StatusCode)
	}
}
