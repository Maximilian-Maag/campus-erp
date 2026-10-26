package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHTTPKLRFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	// Cost centre.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "alice",
		`{"number":"CC-`+uniqueSuffix(t)+`","name":"Bibliothek"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create centre = %d", resp.StatusCode)
	}
	var center struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&center); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if center.ID == 0 {
		t.Fatal("no id")
	}

	// Cost unit.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-units", "alice",
		`{"number":"CU-`+uniqueSuffix(t)+`","name":"Lehre"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create unit = %d", resp.StatusCode)
	}
	var unit struct{ ID int64 }
	_ = json.NewDecoder(resp.Body).Decode(&unit)

	// Records.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(center.ID)+"/records", "alice",
		`{"cost_unit_id":`+itoa(unit.ID)+`,"kind":"cost","amount_cents":30000,"reference":"RE-1"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("post cost = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(center.ID)+"/records", "alice",
		`{"kind":"revenue","amount_cents":50000,"reference":"ER-1"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("post revenue = %d", resp.StatusCode)
	}

	// Report.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/"+itoa(center.ID)+"/report", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("report = %d", resp.StatusCode)
	}
	var rep struct {
		CostCenterID  int64
		CostsCents    int64
		RevenuesCents int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if rep.CostsCents != 30000 || rep.RevenuesCents != 50000 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestHTTPKLRErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Validation (no name) -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "a",
		`{"number":"X","name":""}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation = %d, want 422", resp.StatusCode)
	}
	// Duplicate -> 409.
	num := "CC-DUP-" + uniqueSuffix(t)
	body := `{"number":"` + num + `","name":"n"}`
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "a", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "a", body); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", resp.StatusCode)
	}
	// Unknown centre -> 404.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/9223372036854775807/report", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown report = %d, want 404", resp.StatusCode)
	}
	// Bad id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/abc/report", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	// Malformed record body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/1/records", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad record body = %d, want 400", resp.StatusCode)
	}
	// Malformed cost-unit body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-units", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad unit body = %d, want 400", resp.StatusCode)
	}
}
