package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func orderPayload(reference, dueDate string) string {
	return `{"reference":"` + reference + `","business_year":2026,"amount_cents":125000,` +
		`"currency":"EUR","purpose":"Stromabschlag","due_date":"` + dueDate + `",` +
		`"recipient":{"name":"Stadtwerke","account_holder":"Stadtwerke","iban":"DE02120300000000202051",` +
		`"bic":"BYLADEM1001","bank_name":"Musterbank"},` +
		`"budget_position":{"year":2026,"chapter":"0710","title":"6810","cost_centre":"KST-1","cost_carrier":"KT-1","allocated_cents":500000}}`
}

type orderJSON struct {
	ID     int64 `json:"ID"`
	Status string
}

func createOrder(t *testing.T, srv string, actor string) orderJSON {
	t.Helper()
	ref := fmt.Sprintf("AO-%d", time.Now().UnixNano())
	resp := doJSON(t, http.MethodPost, srv+"/api/payment-orders", actor, orderPayload(ref, "2026-04-01"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create order = %d", resp.StatusCode)
	}
	var o orderJSON
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	return o
}

func TestHTTPPaymentOrderWorkflow(t *testing.T) {
	srv, pool := newTestServer(t)
	order := createOrder(t, srv.URL, "alice")
	base := fmt.Sprintf("%s/api/payment-orders/%d", srv.URL, order.ID)

	if order.Status != "draft" {
		t.Fatalf("new order status = %q, want draft", order.Status)
	}

	// submit
	resp := doJSON(t, http.MethodPost, base+"/submit", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit = %d", resp.StatusCode)
	}

	// the creator must not be able to approve -> 403
	if resp := doJSON(t, http.MethodPost, base+"/approve", "alice", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("self-approval = %d, want 403", resp.StatusCode)
	}

	// an independent approver succeeds
	resp = doJSON(t, http.MethodPost, base+"/approve", "bob", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d", resp.StatusCode)
	}
	var approved orderJSON
	if err := json.NewDecoder(resp.Body).Decode(&approved); err != nil {
		t.Fatalf("decode approved: %v", err)
	}
	if approved.Status != "approved" {
		t.Fatalf("status = %q, want approved", approved.Status)
	}

	// a second approval is a conflict
	if resp := doJSON(t, http.MethodPost, base+"/approve", "carol", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("double approve = %d, want 409", resp.StatusCode)
	}

	// audit: create + submit + approve = 3 events for this order
	subject := fmt.Sprintf("order/%d", order.ID)
	if got := countSubject(t, pool, subject); got != 3 {
		t.Fatalf("audit events for %s = %d, want 3", subject, got)
	}
}

func TestHTTPPaymentOrderErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// malformed body
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-orders", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// bad due date
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-orders", "alice",
		orderPayload(fmt.Sprintf("AO-%d", time.Now().UnixNano()), "01.04.2026")); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad due_date = %d, want 400", resp.StatusCode)
	}
	// validation failure: zero amount
	ref := fmt.Sprintf("AO-%d", time.Now().UnixNano())
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-orders", "alice",
		`{"reference":"`+ref+`","business_year":2026,"amount_cents":0,"currency":"EUR","purpose":"x","due_date":"2026-04-01",`+
			`"recipient":{"name":"n","account_holder":"h","iban":"DE02120300000000202051","bic":"BYLADEM1001","bank_name":"b"},`+
			`"budget_position":{"year":2026,"chapter":"c","title":"t","cost_centre":"","cost_carrier":""}}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("zero amount = %d, want 422", resp.StatusCode)
	}

	order := createOrder(t, srv.URL, "alice")
	base := fmt.Sprintf("%s/api/payment-orders/%d", srv.URL, order.ID)

	// rejecting a draft is a conflict
	if resp := doJSON(t, http.MethodPost, base+"/reject", "bob", `{"reason":"x"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("reject draft = %d, want 409", resp.StatusCode)
	}
	// submitting twice is a conflict
	if resp := doJSON(t, http.MethodPost, base+"/submit", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/submit", "alice", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("double submit = %d, want 409", resp.StatusCode)
	}
	// rejection without a reason is a validation error
	if resp := doJSON(t, http.MethodPost, base+"/reject", "bob", `{"reason":"   "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank reason = %d, want 422", resp.StatusCode)
	}
	// rejection with a reason succeeds
	if resp := doJSON(t, http.MethodPost, base+"/reject", "bob", `{"reason":"kein Budget"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("reject = %d", resp.StatusCode)
	}
	// unknown order
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-orders/9223372036854775807", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order = %d, want 404", resp.StatusCode)
	}
	// non-integer id
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-orders/abc", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	// malformed reject body
	if resp := doJSON(t, http.MethodPost, base+"/reject", "bob", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad reject body = %d, want 400", resp.StatusCode)
	}
	// cancel a draft
	draft := createOrder(t, srv.URL, "alice")
	if resp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/api/payment-orders/%d/cancel", srv.URL, draft.ID), "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel = %d, want 200", resp.StatusCode)
	}
}
