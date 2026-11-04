package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestHTTPKostentraegerRun drives the Kostenträgerrechnung over HTTP: a cost centre's period
// costs are attributed to its cost units, and the run cannot be repeated over the same period.
func TestHTTPKostentraegerRun(t *testing.T) {
	srv, _ := newTestServer(t)

	centerID := createCenter(t, srv.URL, "alice", "K")
	unitID := createUnit(t, srv.URL, "alice", "T")

	// A cost in the period to attribute.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(centerID)+"/records", "alice",
		`{"kind":"cost","amount_cents":100000,"business_date":"2026-04-10"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post record = %d", resp.StatusCode)
	}

	keysURL := srv.URL + "/api/klr/cost-centers/" + itoa(centerID) + "/cost-unit-keys"
	resp = doJSON(t, http.MethodPost, keysURL, "alice",
		`{"target_unit_id":`+itoa(unitID)+`,"share_bp":10000,"valid_from":"2026-01-01"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("define key = %d, want 201", resp.StatusCode)
	}
	var key struct {
		ID           int64
		TargetUnitID int64
		ShareBP      int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&key); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if key.ID == 0 || key.TargetUnitID != unitID || key.ShareBP != 10000 {
		t.Fatalf("key = %+v", key)
	}

	// The list a screen reads before it can pick a unit at all.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-units", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list units = %d", resp.StatusCode)
	}
	var units []struct {
		ID     int64
		Number string
	}
	if err := json.NewDecoder(resp.Body).Decode(&units); err != nil {
		t.Fatalf("decode units: %v", err)
	}
	found := false
	for _, u := range units {
		if u.ID == unitID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the new cost unit is missing from the list of %d", len(units))
	}

	// The list a screen reads before it can run anything.
	resp = doJSON(t, http.MethodGet, keysURL, "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list keys = %d", resp.StatusCode)
	}
	var keys []struct {
		ID           int64
		TargetUnitID int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	if len(keys) != 1 || keys[0].TargetUnitID != unitID {
		t.Fatalf("keys = %+v", keys)
	}

	runURL := keysURL + "/run"
	resp = doJSON(t, http.MethodPost, runURL, "alice", `{"from":"2026-04-01","to":"2026-04-30"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("run = %d, want 201", resp.StatusCode)
	}
	var run struct {
		ID             int64
		AllocatedCents int64
		Receivers      int
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.AllocatedCents != 100000 || run.Receivers != 1 {
		t.Fatalf("run = %+v", run)
	}

	// The same period cannot be attributed twice, nor can one overlapping it.
	if resp := doJSON(t, http.MethodPost, runURL, "alice", `{"from":"2026-04-01","to":"2026-04-30"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second run = %d, want 409", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, runURL, "alice", `{"from":"2026-04-15","to":"2026-05-15"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("overlapping run = %d, want 409", resp.StatusCode)
	}
}

// TestHTTPCostUnitKeyRefusals: the statuses a screen reads for the mistakes it can make.
func TestHTTPCostUnitKeyRefusals(t *testing.T) {
	srv, _ := newTestServer(t)

	centerID := createCenter(t, srv.URL, "alice", "K")
	keysURL := srv.URL + "/api/klr/cost-centers/" + itoa(centerID) + "/cost-unit-keys"

	if resp := doJSON(t, http.MethodPost, keysURL, "alice",
		`{"target_unit_id":9223372036854775807,"share_bp":10000,"valid_from":"2026-01-01"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown unit = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL, "alice",
		`{"target_unit_id":1,"share_bp":10001,"valid_from":"2026-01-01"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("share out of range = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL, "alice",
		`{"target_unit_id":1,"share_bp":10000,"valid_from":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_from = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL, "alice",
		`{"target_unit_id":1,"share_bp":10000,"valid_from":"2026-01-01","valid_to":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_to = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/9223372036854775807/cost-unit-keys", "alice", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown centre = %d, want 404", resp.StatusCode)
	}
	// A run over a period whose key is not in force has nothing to attribute.
	if resp := doJSON(t, http.MethodPost, keysURL+"/run", "alice", `{"from":"2026-04-01","to":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("run with no keys = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL+"/run", "alice", `{"from":"nonsense","to":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad from = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL+"/run", "alice", `{"from":"2026-04-01","to":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad to = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, keysURL+"/run", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}
}

// createUnit makes a cost unit over the API and returns its id.
func createUnit(t *testing.T, srvURL, actor, prefix string) int64 {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srvURL+"/api/klr/cost-units", actor,
		`{"number":"`+prefix+`-`+uniqueSuffix(t)+`","name":"`+prefix+`"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create unit = %d", resp.StatusCode)
	}
	var unit struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&unit); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	return unit.ID
}
