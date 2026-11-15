package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
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
	AvailableCents int64 `json:"available_cents"`
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
	// The API states what is still free, so a client does not re-implement the guard's rule.
	if view.AvailableCents != 70000 {
		t.Fatalf("available = %d, want the position less the commitment", view.AvailableCents)
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

	// The list is what a client's Mittelübersicht shows: every position, each with its balance
	// already read, so the screen does not have to ask position by position.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list positions = %d", resp.StatusCode)
	}
	var list []budgetPositionJSON
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var found bool
	for _, p := range list {
		if p.Position.ID == pos.ID {
			found = true
			if p.Balance.AllocatedCents != 100000 || p.Balance.CommittedCents != 30000 {
				t.Fatalf("listed balance = %+v", p.Balance)
			}
		}
	}
	if !found {
		t.Fatalf("the created position is not in the list of %d", len(list))
	}
}

// stubBudgetBalanceFails lists positions but cannot read a balance: the list must fail rather
// than render a budget with a missing figure.
type stubBudgetBalanceFails struct{ stubBudget }

func (s stubBudgetBalanceFails) Positions(context.Context, int, int) ([]budget.Position, error) {
	return []budget.Position{{ID: 1, Year: 2026, Chapter: "1", Title: "2"}}, nil
}

// TestListBudgetPositionsFailurePaths: neither a position list that cannot be read nor a
// balance that cannot be read may produce a half-rendered budget.
func TestListBudgetPositionsFailurePaths(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name string
		deps httpapi.Deps
	}{
		{"positions fail", httpapi.Deps{Budget: stubBudget{err: boom}}},
		{"balance fails", httpapi.Deps{Budget: stubBudgetBalanceFails{stubBudget{err: boom}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(httpapi.New(c.deps).Routes())
			t.Cleanup(srv.Close)

			if resp := doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions", "alice", ""); resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", resp.StatusCode)
			}
		})
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

// TestHTTPBudgetPositionsPage: the list answers a page — the default when the client does not say,
// exactly what was asked for when it does — and refuses a page that is not a non-negative integer
// with the machine code a client branches on. A page past the end is empty, not an error.
func TestHTTPBudgetPositionsPage(t *testing.T) {
	srv, _ := newTestServer(t)

	// Three positions of this test's own, so the walk below spans several of them whatever else the
	// shared schema holds.
	tag := uniqueSuffix(t)
	for _, title := range []string{"1", "2", "3"} {
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions", "alice",
			`{"year":2026,"chapter":"PG","title":"`+tag+`-`+title+`","description":"Paging","allocated_cents":1000}`)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create position %s = %d", title, resp.StatusCode)
		}
	}
	base := srv.URL + "/api/budget/positions"

	// The default page is the first one: the request without parameters is a page, not the table.
	resp := doJSON(t, http.MethodGet, base, "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("default page = %d", resp.StatusCode)
	}
	var all []budgetPositionJSON
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		t.Fatalf("decode default page: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("the default page is empty, so nothing about paging is being tested")
	}

	// Two pages of two reproduce the first four rows of the same list, in order.
	var first, second []budgetPositionJSON
	resp = doJSON(t, http.MethodGet, base+"?limit=2", "alice", "")
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	resp = doJSON(t, http.MethodGet, base+"?limit=2&offset=2", "alice", "")
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("a page of 2 holds %d rows", len(first))
	}
	for i := range first {
		if first[i].Position.ID != all[i].Position.ID {
			t.Fatalf("the first page diverges from the list at %d: got %d, want %d",
				i, first[i].Position.ID, all[i].Position.ID)
		}
	}
	for i := range second {
		if second[i].Position.ID != all[2+i].Position.ID {
			t.Fatalf("the second page diverges from the list at %d: got %d, want %d",
				i, second[i].Position.ID, all[2+i].Position.ID)
		}
	}

	// A page past the end is empty rather than an error.
	resp = doJSON(t, http.MethodGet, base+"?offset=100000", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page past the end = %d, want 200", resp.StatusCode)
	}
	var past []budgetPositionJSON
	if err := json.NewDecoder(resp.Body).Decode(&past); err != nil {
		t.Fatalf("decode past the end: %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows", len(past))
	}

	for _, tc := range []struct{ query, code string }{
		{"?limit=0", "invalid_limit"},
		{"?limit=f%C3%BCnf", "invalid_limit"},
		{"?offset=zw%C3%B6lf", "invalid_offset"},
		{"?offset=-1", "invalid_offset"},
	} {
		resp = doJSON(t, http.MethodGet, base+tc.query, "alice", "")
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want 422", tc.query, resp.StatusCode)
		}
		var problem struct{ Code string }
		if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
			t.Fatalf("%s: decode problem: %v", tc.query, err)
		}
		if problem.Code != tc.code {
			t.Fatalf("%s: code = %q, want %q", tc.query, problem.Code, tc.code)
		}
	}
}
