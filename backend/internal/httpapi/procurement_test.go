package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

func newBudgetPosition(t *testing.T, srv string) int64 {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv+"/api/budget/positions", "alice",
		`{"year":2026,"chapter":"05","title":"PO-`+uniqueSuffix(t)+`","allocated_cents":100000}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create position = %d", resp.StatusCode)
	}
	var pos struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&pos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return pos.ID
}

func poBody(number string, positionID int64) string {
	return `{"number":"` + number + `","supplier":"Lieferant GmbH","budget_position_id":` + itoa(positionID) + `,"currency":"EUR",` +
		`"lines":[{"line_no":1,"description":"Ware","quantity_milli":1000,"unit_price_cents":10000,"net_cents":10000,"vat_category":"S","vat_rate_bp":1900}]}`
}

func TestHTTPProcurementFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	posID := newBudgetPosition(t, srv.URL)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "alice", poBody("PO-"+uniqueSuffix(t), posID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	var order struct {
		ID         int64
		GrossCents int64
		Status     string
	}
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if order.GrossCents != 11900 || order.Status != "draft" {
		t.Fatalf("order = %+v", order)
	}

	// Approve.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders/"+itoa(order.ID)+"/approve", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d", resp.StatusCode)
	}
	var approved struct{ Status string }
	_ = json.NewDecoder(resp.Body).Decode(&approved)
	if approved.Status != "approved" {
		t.Fatalf("status = %s", approved.Status)
	}

	// The budget position now shows the commitment (cross-module over HTTP).
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/"+itoa(posID)+"/balance", "alice", "")
	var bal struct{ CommittedCents int64 }
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if bal.CommittedCents != 11900 {
		t.Fatalf("committed = %d, want 11900", bal.CommittedCents)
	}

	// Get + receive.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders/"+itoa(order.ID), "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("get = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders/"+itoa(order.ID)+"/receive", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("receive = %d", resp.StatusCode)
	}
}

// TestHTTPProcurementList: the list a screen reads — every order as a summary, and a status
// filter that refuses a status outside the lifecycle instead of quietly listing everything.
func TestHTTPProcurementList(t *testing.T) {
	srv, _ := newTestServer(t)
	posID := newBudgetPosition(t, srv.URL)

	draftNumber := "PO-L1-" + uniqueSuffix(t)
	approvedNumber := "PO-L2-" + uniqueSuffix(t)
	var draft, approved struct {
		ID         int64
		GrossCents int64
		Status     string
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "alice", poBody(draftNumber, posID)); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create draft = %d", resp.StatusCode)
	} else if err := json.NewDecoder(resp.Body).Decode(&draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "alice", poBody(approvedNumber, posID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second = %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&approved); err != nil {
		t.Fatalf("decode approved: %v", err)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders/"+itoa(approved.ID)+"/approve", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d", resp.StatusCode)
	}

	type summary struct {
		ID         int64
		Number     string
		Supplier   string
		GrossCents int64
		Status     string
	}
	list := func(query string) []summary {
		t.Helper()
		resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders"+query, "alice", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list%s = %d", query, resp.StatusCode)
		}
		var out []summary
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		return out
	}

	all := list("")
	byID := map[int64]summary{}
	for _, s := range all {
		byID[s.ID] = s
	}
	got, ok := byID[draft.ID]
	if !ok {
		t.Fatalf("draft %d missing from %d summaries", draft.ID, len(all))
	}
	if got.Number != draftNumber || got.Supplier != "Lieferant GmbH" || got.Status != "draft" || got.GrossCents != 11900 {
		t.Fatalf("summary = %+v", got)
	}

	onlyApproved := list("?status=approved")
	if len(onlyApproved) == 0 {
		t.Fatal("the approved order is missing from the filtered list")
	}
	for _, s := range onlyApproved {
		if s.Status != "approved" {
			t.Fatalf("filter returned a %s order", s.Status)
		}
		if s.ID == draft.ID {
			t.Fatal("the draft appeared in the approved filter")
		}
	}

	// An unknown status is refused, not ignored.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders?status=nope", "alice", ""); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown status = %d, want 422", resp.StatusCode)
	}
}

// TestProcurementListFailure: a list that cannot be read is a 500, not an empty list that looks
// like "there are no orders".
func TestProcurementListFailure(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Procurement: stubProcurement{err: errors.New("boom")}}).Routes())
	t.Cleanup(srv.Close)

	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders", "alice", ""); resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestHTTPProcurementErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	posID := newBudgetPosition(t, srv.URL)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Validation failure (no supplier) -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "a",
		`{"number":"X","supplier":"","budget_position_id":`+itoa(posID)+`,"currency":"EUR","lines":[]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation = %d, want 422", resp.StatusCode)
	}
	// Unknown order -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders/9223372036854775807", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", resp.StatusCode)
	}
	// Bad transition (receive a draft) -> 409.
	num := "PO-T-" + uniqueSuffix(t)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "a", poBody(num, posID))
	var order struct{ ID int64 }
	_ = json.NewDecoder(resp.Body).Decode(&order)
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders/"+itoa(order.ID)+"/receive", "a", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("bad transition = %d, want 409", resp.StatusCode)
	}
	// Duplicate number -> 409.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "a", poBody(num, posID)); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", resp.StatusCode)
	}
	// Bad id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/purchase-orders/abc", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
}
