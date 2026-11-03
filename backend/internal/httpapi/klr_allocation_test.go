package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// createCenter makes a cost centre over the API and returns its id.
func createCenter(t *testing.T, srvURL, actor, prefix string) int64 {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srvURL+"/api/klr/cost-centers", actor,
		`{"number":"`+prefix+`-`+uniqueSuffix(t)+`","name":"`+prefix+`"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create centre = %d", resp.StatusCode)
	}
	var center struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&center); err != nil {
		t.Fatalf("decode centre: %v", err)
	}
	return center.ID
}

// TestHTTPAllocationFlow: define the keys of a cost centre, read them back, distribute a
// period's costs over them, and find the receivers charged.
func TestHTTPAllocationFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	source := createCenter(t, srv.URL, "alice", "SRC")
	receiver := createCenter(t, srv.URL, "alice", "RCV")

	// A record with an explicit period, and one without (which is today's).
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/records", "alice",
		`{"kind":"cost","amount_cents":80000,"business_date":"2026-04-10","reference":"RE-Apr"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("post cost = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/records", "alice",
		`{"kind":"cost","amount_cents":1000,"reference":"RE-today"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("post cost without a date = %d", resp.StatusCode)
	}

	// The key.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/allocations", "alice",
		`{"target_center_id":`+itoa(receiver)+`,"share_bp":10000,"valid_from":"2026-04-01","valid_to":"2026-04-30"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("define allocation = %d", resp.StatusCode)
	}
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/allocations", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list allocations = %d", resp.StatusCode)
	}
	var keys []struct {
		ID             int64
		SourceCenterID int64
		TargetCenterID int64
		ShareBP        int64
		ValidTo        *string
	}
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	if len(keys) != 1 || keys[0].SourceCenterID != source || keys[0].TargetCenterID != receiver || keys[0].ShareBP != 10000 {
		t.Fatalf("keys = %+v", keys)
	}

	// The run.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/allocations/run", "carol",
		`{"from":"2026-04-01","to":"2026-04-30"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("run = %d", resp.StatusCode)
	}
	var run struct {
		ID             int64
		SourceCenterID int64
		AllocatedCents int64
		Receivers      int
		RunBy          string
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	// Only the April cost is distributed; today's cost is not part of that period.
	if run.ID == 0 || run.AllocatedCents != 80000 || run.Receivers != 1 || run.RunBy != "carol" {
		t.Fatalf("run = %+v", run)
	}

	// The receiver carries it.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/"+itoa(receiver)+"/report", "alice", "")
	var report struct{ CostsCents int64 }
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.CostsCents != 80000 {
		t.Fatalf("receiver costs = %d, want 80000", report.CostsCents)
	}
}

// TestHTTPAllocationErrorBranches: the ways a key or a run is refused.
func TestHTTPAllocationErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	source := createCenter(t, srv.URL, "alice", "SRC-E")
	receiver := createCenter(t, srv.URL, "alice", "RCV-E")
	path := srv.URL + "/api/klr/cost-centers/" + itoa(source) + "/allocations"

	// A key is stored for the range it names; a bad date is a 422, a bad body a 400, a bad id a
	// 400, and an unknown receiver a 404.
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":`+itoa(receiver)+`,"share_bp":10000,"valid_from":"1.4.2026"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_from = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":`+itoa(receiver)+`,"share_bp":10000,"valid_from":"2026-04-01","valid_to":"April"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_to = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, path, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/abc/allocations", "alice",
		`{"target_center_id":1,"share_bp":1,"valid_from":"2026-04-01"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":9223372036854775807,"share_bp":10000,"valid_from":"2026-04-01"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown target = %d, want 404", resp.StatusCode)
	}
	// A centre distributing to itself, and a share that is not a share.
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":`+itoa(source)+`,"share_bp":10000,"valid_from":"2026-04-01"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("self = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":`+itoa(receiver)+`,"share_bp":10001,"valid_from":"2026-04-01"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("share out of range = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/abc/allocations", "alice", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id on list = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/klr/cost-centers/9223372036854775807/allocations", "alice", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown centre on list = %d, want 404", resp.StatusCode)
	}

	// A run: bad dates, a bad body, a bad id, and the refusals the engine makes.
	runPath := srv.URL + "/api/klr/cost-centers/" + itoa(source) + "/allocations/run"
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{"from":"April","to":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad from = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{"from":"2026-04-01","to":"April"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad to = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad run body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/abc/allocations/run", "alice",
		`{"from":"2026-04-01","to":"2026-04-30"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id on run = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/9223372036854775807/allocations/run", "alice",
		`{"from":"2026-04-01","to":"2026-04-30"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown centre on run = %d, want 404", resp.StatusCode)
	}
	// No key in force yet.
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{"from":"2026-12-01","to":"2026-12-31"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("run without keys = %d, want 422", resp.StatusCode)
	}
	// A key in force, and costs in the period: the run succeeds, and a second one is refused.
	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"target_center_id":`+itoa(receiver)+`,"share_bp":10000,"valid_from":"2026-12-01"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("define = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/cost-centers/"+itoa(source)+"/records", "alice",
		`{"kind":"cost","amount_cents":5000,"business_date":"2026-12-05","reference":"RE-Dec"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("post = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{"from":"2026-12-01","to":"2026-12-31"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("run = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, runPath, "alice", `{"from":"2026-12-01","to":"2026-12-31"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second run = %d, want 409", resp.StatusCode)
	}
}

// TestHTTPRecordBusinessDate: a record's period is the caller's to state, and a malformed one is
// refused rather than guessed.
func TestHTTPRecordBusinessDate(t *testing.T) {
	srv, _ := newTestServer(t)
	center := createCenter(t, srv.URL, "alice", "SRC-D")
	path := srv.URL + "/api/klr/cost-centers/" + itoa(center) + "/records"

	resp := doJSON(t, http.MethodPost, path, "alice",
		`{"kind":"cost","amount_cents":100,"business_date":"2026-04-30","reference":"RE"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post = %d", resp.StatusCode)
	}
	var rec struct {
		BusinessDate string
		AmountCents  int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.BusinessDate[:10] != "2026-04-30" || rec.AmountCents != 100 {
		t.Fatalf("record = %+v", rec)
	}

	if resp := doJSON(t, http.MethodPost, path, "alice",
		`{"kind":"cost","amount_cents":100,"business_date":"30.04.2026"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad business date = %d, want 422", resp.StatusCode)
	}
}
