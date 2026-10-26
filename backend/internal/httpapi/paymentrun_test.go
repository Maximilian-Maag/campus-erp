package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type accountJSON struct {
	ID   int64
	IBAN string
}

type runOrderJSON struct {
	ID        int64
	Reference string
	Status    string
}

type runJSON struct {
	ID        int64
	Reference string
	ItemCount int
	SumCents  int64
	FileHash  string
}

// createApprovedOrder drives one order through create -> submit -> approve over
// HTTP and returns its id.
func createApprovedOrder(t *testing.T, srv string, amountCents int64) runOrderJSON {
	t.Helper()
	body := `{"reference":"AO-` + uniqueSuffix(t) + `","business_year":2026,"amount_cents":` + itoa(amountCents) + `,
		"currency":"EUR","purpose":"Stromabschlag","due_date":"2026-04-01",
		"recipient":{"name":"Empfänger","account_holder":"Empfänger GmbH","iban":"DE89370400440532013000","bic":"COBADEFFXXX","bank_name":"Commerzbank"},
		"budget_position":{"year":2026,"chapter":"0710","title":"6810","cost_centre":"KST-` + uniqueSuffix(t) + `","cost_carrier":"KT"}}`
	resp := doJSON(t, http.MethodPost, srv+"/api/payment-orders", "alice", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create order = %d", resp.StatusCode)
	}
	var o runOrderJSON
	if err := json.NewDecoder(resp.Body).Decode(&o); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if resp := doJSON(t, http.MethodPost, srv+"/api/payment-orders/"+itoa(o.ID)+"/submit", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit = %d", resp.StatusCode)
	}
	// The approver must differ from creator/submitter.
	resp = doJSON(t, http.MethodPost, srv+"/api/payment-orders/"+itoa(o.ID)+"/approve", "bob", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d", resp.StatusCode)
	}
	return o
}

func TestHTTPPaymentRunFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	// Bank account.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/cash-accounts", "carol",
		`{"name":"Universität Musterstadt","iban":"DE02120300000000202051","bic":"BYLADEM1001","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create cash account = %d", resp.StatusCode)
	}
	var account accountJSON
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	a := createApprovedOrder(t, srv.URL, 125000)
	b := createApprovedOrder(t, srv.URL, 4999)

	ref := "RUN-" + uniqueSuffix(t)
	runBody := `{"reference":"` + ref + `","account_id":` + itoa(account.ID) +
		`,"execution_date":"2026-10-28","order_ids":[` + itoa(a.ID) + `,` + itoa(b.ID) + `]}`
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "carol", runBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run = %d", resp.StatusCode)
	}
	var run runJSON
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.ItemCount != 2 || run.SumCents != 129999 || run.FileHash == "" {
		t.Fatalf("run = %+v", run)
	}

	// Fetch the run.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/"+itoa(run.ID), "carol", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("get run = %d", resp.StatusCode)
	}

	// Fetch the file.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/"+itoa(run.ID)+"/file", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run file = %d", resp.StatusCode)
	}
	file, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(file), "<MsgId>"+ref+"</MsgId>") {
		t.Fatalf("file does not contain the message id:\n%s", file)
	}

	// Idempotent re-run: same reference returns the same run.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "carol", runBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("re-run = %d", resp.StatusCode)
	}
	var again runJSON
	if err := json.NewDecoder(resp.Body).Decode(&again); err != nil {
		t.Fatalf("decode re-run: %v", err)
	}
	if again.ID != run.ID {
		t.Fatalf("re-run id = %d, want %d", again.ID, run.ID)
	}
}

func TestHTTPPaymentRunErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Bad execution date.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "a",
		`{"reference":"x","account_id":1,"execution_date":"28.10.2026","order_ids":[1]}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad date = %d, want 400", resp.StatusCode)
	}
	// No orders -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "a",
		`{"reference":"RUN-`+uniqueSuffix(t)+`","account_id":1,"execution_date":"2026-10-28","order_ids":[]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no orders = %d, want 422", resp.StatusCode)
	}
	// Unknown run -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/9223372036854775807", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown run = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/9223372036854775807/file", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown run file = %d, want 404", resp.StatusCode)
	}
	// Non-integer id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/abc", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad run id = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-runs/abc/file", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad run file id = %d, want 400", resp.StatusCode)
	}
	// Bad cash account IBAN -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cash-accounts", "a",
		`{"name":"Uni","iban":"DE00","bic":"BYLADEM1001","currency":"EUR"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad iban = %d, want 422", resp.StatusCode)
	}
	// Malformed bank-account body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cash-accounts", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad account body = %d, want 400", resp.StatusCode)
	}
}
