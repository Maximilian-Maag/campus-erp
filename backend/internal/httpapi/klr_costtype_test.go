package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// TestHTTPCostTypeBridge drives the cost-type catalogue and the account bridge over HTTP: the
// status a screen reads, not just the handler's own return.
func TestHTTPCostTypeBridge(t *testing.T) {
	srv, _ := newTestServer(t)
	mark := uniqueSuffix(t)
	base := srv.URL + "/api/klr/cost-types"

	personal := createCostType(t, base, "4711-"+mark, "Personalkosten")
	sach := createCostType(t, base, "5000-"+mark, "Sachkosten")

	// The code is the identity of a cost type.
	if resp := doJSON(t, http.MethodPost, base, "alice",
		`{"code":"4711-`+mark+`","name":"Doppelt"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate code = %d, want 409", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base, "alice",
		`{"code":" ","name":"x"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank code = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}

	// The list a screen reads before it can ask about a cost type.
	resp := doJSON(t, http.MethodGet, base, "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list cost types = %d", resp.StatusCode)
	}
	var types []struct {
		ID   int64
		Code string
	}
	if err := json.NewDecoder(resp.Body).Decode(&types); err != nil {
		t.Fatalf("decode cost types: %v", err)
	}
	var havePersonal, haveSach bool
	for _, ct := range types {
		if ct.ID == personal {
			havePersonal = true
		}
		if ct.ID == sach {
			haveSach = true
		}
	}
	if !havePersonal || !haveSach {
		t.Fatalf("the new cost types are missing from the list of %d", len(types))
	}

	// The bridge: an account belongs to exactly one cost type.
	account := "6810-" + mark
	accountsURL := base + "/" + strconv.FormatInt(personal, 10) + "/accounts"
	if resp := doJSON(t, http.MethodPost, accountsURL, "alice",
		`{"account_code":"`+account+`"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("map account = %d, want 201", resp.StatusCode)
	}
	// The same mapping again is idempotent, another cost type is refused.
	if resp := doJSON(t, http.MethodPost, accountsURL, "alice",
		`{"account_code":"`+account+`"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("re-map account = %d, want 201", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/"+strconv.FormatInt(sach, 10)+"/accounts", "alice",
		`{"account_code":"`+account+`"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second mapping = %d, want 409", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/9223372036854775807/accounts", "alice",
		`{"account_code":"x-`+mark+`"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cost type = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, accountsURL, "alice",
		`{"account_code":" "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank account = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, accountsURL, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}

	// The bridge reads both ways.
	resp = doJSON(t, http.MethodGet, accountsURL, "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list accounts = %d", resp.StatusCode)
	}
	var links []struct {
		CostTypeID  int64
		AccountCode string
	}
	if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	found := false
	for _, l := range links {
		if l.AccountCode == account && l.CostTypeID == personal {
			found = true
		}
	}
	if !found {
		t.Fatalf("the account is not listed under the cost type (%d links)", len(links))
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/klr/accounts/"+account+"/cost-type", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("by-account = %d, want 200", resp.StatusCode)
	}
	var byAccount struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&byAccount); err != nil {
		t.Fatalf("decode by-account: %v", err)
	}
	if byAccount.ID != personal {
		t.Fatalf("by-account = %+v, want %d", byAccount, personal)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/accounts/unmapped-"+mark+"/cost-type", "alice", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unmapped account = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, base+"/9223372036854775807/accounts", "alice", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cost type accounts = %d, want 404", resp.StatusCode)
	}
}

// TestHTTPRecordCarriesItsCostType: the posting route accepts a cost type and hands it back.
func TestHTTPRecordCarriesItsCostType(t *testing.T) {
	srv, _ := newTestServer(t)
	mark := uniqueSuffix(t)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers", "alice",
		`{"number":"CC-`+mark+`","name":"Bibliothek"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create centre = %d", resp.StatusCode)
	}
	var center struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&center); err != nil {
		t.Fatalf("decode centre: %v", err)
	}

	costType := createCostType(t, srv.URL+"/api/klr/cost-types", "4711-"+mark, "Personalkosten")

	recordsURL := srv.URL + "/api/klr/cost-centers/" + strconv.FormatInt(center.ID, 10) + "/records"
	resp = doJSON(t, http.MethodPost, recordsURL, "alice",
		`{"kind":"cost","amount_cents":1250,"cost_type_id":`+strconv.FormatInt(costType, 10)+`}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post record = %d, want 201", resp.StatusCode)
	}
	var rec struct{ CostTypeID *int64 }
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	if rec.CostTypeID == nil || *rec.CostTypeID != costType {
		t.Fatalf("record cost type = %v, want %d", rec.CostTypeID, costType)
	}

	// A record may carry no cost type.
	resp = doJSON(t, http.MethodPost, recordsURL, "alice", `{"kind":"cost","amount_cents":100}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post record without a cost type = %d, want 201", resp.StatusCode)
	}

	// A cost type that does not exist is refused.
	resp = doJSON(t, http.MethodPost, recordsURL, "alice",
		`{"kind":"cost","amount_cents":100,"cost_type_id":9223372036854775807}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cost type on a record = %d, want 404", resp.StatusCode)
	}
}

// createCostType creates a cost type and returns its id.
func createCostType(t *testing.T, base, code, name string) int64 {
	t.Helper()
	resp := doJSON(t, http.MethodPost, base, "alice", `{"code":"`+code+`","name":"`+name+`"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create cost type %s = %d", code, resp.StatusCode)
	}
	var ct struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&ct); err != nil {
		t.Fatalf("decode cost type: %v", err)
	}
	if ct.ID == 0 {
		t.Fatal("cost type has no id")
	}
	return ct.ID
}
