package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type einvoiceJSON struct {
	ID     int64
	Number string
	Lines  []struct {
		LineNo   int
		NetCents int64
	}
}

func einvoiceBody(number string) string {
	return `{"number":"` + number + `","issue_date":"2026-10-26","type_code":"380","currency":"EUR",` +
		`"buyer_reference":"04011000-12345-03",` +
		`"seller":{"name":"Universität Musterstadt","street":"Musterstraße 1","city":"Musterstadt","postal_code":"12345","country":"DE","vat_id":"DE123456789","endpoint_id":"rechnung@uni-musterstadt.de","endpoint_scheme":"EM","email":"rechnung@uni-musterstadt.de"},` +
		`"buyer":{"name":"Stadt Musterstadt","street":"Rathausplatz 1","city":"Musterstadt","postal_code":"12345","country":"DE"},` +
		`"payment_means_code":"58","payee_iban":"DE75512108001245126199","payment_terms":"Zahlbar in 30 Tagen.",` +
		`"lines":[` +
		`{"line_no":1,"description":"Druckleistung","quantity_milli":1000,"unit_code":"C62","unit_price_cents":28879,"net_cents":28879,"vat_category":"S","vat_rate_bp":700},` +
		`{"line_no":2,"description":"Literatur","quantity_milli":2000,"unit_code":"C62","unit_price_cents":1304,"net_cents":2608,"vat_category":"S","vat_rate_bp":1900}` +
		`]}`
}

func TestHTTPEInvoiceFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "alice", einvoiceBody("RE-"+uniqueSuffix(t)))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	var inv einvoiceJSON
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inv.ID == 0 || len(inv.Lines) != 2 {
		t.Fatalf("invoice = %+v", inv)
	}

	// Get.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/e-invoices/"+itoa(inv.ID), "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get = %d", resp.StatusCode)
	}

	// Render XRechnung.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/e-invoices/"+itoa(inv.ID)+"/xrechnung", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("render = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("content-type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	doc := string(body)
	if !strings.Contains(doc, "<ubl:Invoice") || !strings.Contains(doc, inv.Number) {
		t.Fatalf("document = %s", doc)
	}
}

func TestHTTPEInvoiceErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Bad date.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "a",
		strings.Replace(einvoiceBody("RE-x"), `"2026-10-26"`, `"26.10.2026"`, 1)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad date = %d, want 400", resp.StatusCode)
	}
	// Validation failure (missing buyer reference) -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "a",
		strings.Replace(einvoiceBody("RE-y"), `"buyer_reference":"04011000-12345-03"`, `"buyer_reference":""`, 1)); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation = %d, want 422", resp.StatusCode)
	}
	// Duplicate number -> 409.
	num := "RE-" + uniqueSuffix(t)
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "a", einvoiceBody(num)); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices", "a", einvoiceBody(num)); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", resp.StatusCode)
	}
	// Unknown id -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/e-invoices/9223372036854775807", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown = %d, want 404", resp.StatusCode)
	}
	// Bad id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/e-invoices/abc/xrechnung", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
}
