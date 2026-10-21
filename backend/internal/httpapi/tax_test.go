package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func uniq(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano()%1000000)
}

func TestHTTPVATTypeFlow(t *testing.T) {
	srv, pool := newTestServer(t)
	code := uniq("T")

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+code+`","description":"Regelsteuersatz","rate_basis_points":1900,"kind":"output"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create VAT type = %d", resp.StatusCode)
	}
	var created struct {
		ID              int64 `json:"ID"`
		Code            string
		RateBasisPoints int64
		Kind            string
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.RateBasisPoints != 1900 || created.Kind != "output" {
		t.Fatalf("created = %+v", created)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/vat-types/"+code, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get VAT type = %d", resp.StatusCode)
	}

	if got := countSubject(t, pool, "vat-type/"+code); got != 1 {
		t.Fatalf("audit events = %d, want 1", got)
	}

	// duplicate -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+code+`","rate_basis_points":1900,"kind":"output"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate VAT type = %d, want 409", resp.StatusCode)
	}
}

func TestHTTPVATTypeErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// unknown kind -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+uniq("T")+`","rate_basis_points":1900,"kind":"sideways"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown kind = %d, want 422", resp.StatusCode)
	}
	// rate out of range
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+uniq("T")+`","rate_basis_points":10001,"kind":"output"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rate out of range = %d, want 422", resp.StatusCode)
	}
	// code too long
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"123456789","rate_basis_points":0,"kind":"output"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("code too long = %d, want 422", resp.StatusCode)
	}
	// unknown code -> 404
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/vat-types/nope", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown VAT code = %d, want 404", resp.StatusCode)
	}
}

func TestHTTPTaxOfficeAndFoundation(t *testing.T) {
	srv, _ := newTestServer(t)
	num := uniq("9")

	// tax office with a valid IBAN
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+num+`","name":"Finanzamt Musterstadt","street":"Steuerweg 1",`+
			`"postal_code":"12345","city":"Musterstadt","account_holder":"Land Muster",`+
			`"iban":"DE02120300000000202051","bic":"byladem1001"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tax office = %d", resp.StatusCode)
	}
	var office struct {
		BIC  string
		IBAN string
	}
	if err := json.NewDecoder(resp.Body).Decode(&office); err != nil {
		t.Fatalf("decode office: %v", err)
	}
	if office.BIC != "BYLADEM1001" {
		t.Fatalf("BIC not upper-cased: %q", office.BIC)
	}

	// invalid IBAN -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+uniq("8")+`","name":"Amt","iban":"DE02120300000000202052"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid IBAN = %d, want 422", resp.StatusCode)
	}
	// duplicate -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+num+`","name":"Amt"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate tax office = %d, want 409", resp.StatusCode)
	}
	// malformed body
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad office body = %d, want 400", resp.StatusCode)
	}
	// bad year -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":1800,"number":"1","name":"Amt"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad year = %d, want 422", resp.StatusCode)
	}

	// foundation
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+uniq("7")+`","description":"Deutschlandstipendium","carry_over":true}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation = %d", resp.StatusCode)
	}
	fnum := uniq("6")
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+fnum+`","description":"Stiftung"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+fnum+`","description":"Stiftung"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate foundation = %d, want 409", resp.StatusCode)
	}
	// blank description -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+uniq("5")+`","description":"   "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank description = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad foundation body = %d, want 400", resp.StatusCode)
	}
}
