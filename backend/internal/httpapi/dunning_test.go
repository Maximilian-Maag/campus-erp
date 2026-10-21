package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func uniqueLevelCode(t *testing.T) string { return "L" + uniqueSuffix(t) }

func levelPayload(code string) string {
	return `{"year":2026,"code":"` + code + `","description":"1. Mahnung","days_allowed":14,` +
		`"active":true,"next_code":"2","bands":[` +
		`{"up_to_cents":5000,"fee_cents":500,"rate_basis_points":0},` +
		`{"up_to_cents":0,"fee_cents":2500,"rate_basis_points":200}]}`
}

type levelJSON struct {
	ID    int64 `json:"ID"`
	Code  string
	Bands []struct {
		UpToCents       int64
		FeeCents        int64
		RateBasisPoints int64
	}
}

func createLevel(t *testing.T, srv, code string) levelJSON {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv+"/api/dunning/levels", "alice", levelPayload(code))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create level = %d", resp.StatusCode)
	}
	var l levelJSON
	if err := json.NewDecoder(resp.Body).Decode(&l); err != nil {
		t.Fatalf("decode level: %v", err)
	}
	return l
}

func TestHTTPDunningFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	code := uniqueLevelCode(t)
	level := createLevel(t, srv.URL, code)

	if len(level.Bands) != 2 {
		t.Fatalf("bands = %d, want 2", len(level.Bands))
	}

	// read it back
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/levels/2026/"+code, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get level = %d", resp.StatusCode)
	}

	// issue a notice: 12000 -> open-ended band, fee 2500 + 2% of 12000 = 240
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/dunning/notices", "alice",
		`{"year":2026,"level_code":"`+code+`","claim_reference":"F-2026-001","outstanding_cents":12000}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("issue notice = %d", resp.StatusCode)
	}
	var notice struct {
		ID          int64 `json:"ID"`
		ChargeCents int64
		LevelCode   string
	}
	if err := json.NewDecoder(resp.Body).Decode(&notice); err != nil {
		t.Fatalf("decode notice: %v", err)
	}
	if notice.ChargeCents != 2740 {
		t.Fatalf("charge = %d, want 2740", notice.ChargeCents)
	}
	if notice.LevelCode != code {
		t.Fatalf("level = %q, want %q", notice.LevelCode, code)
	}
}

func TestHTTPDunningErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	code := uniqueLevelCode(t)

	// malformed bodies
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/levels", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad level body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/notices", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad notice body = %d, want 400", resp.StatusCode)
	}

	// a level without bands is a validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/levels", "alice",
		`{"year":2026,"code":"`+code+`","description":"x","bands":[]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no bands = %d, want 422", resp.StatusCode)
	}
	// descending bands are a validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/levels", "alice",
		`{"year":2026,"code":"`+code+`","bands":[{"up_to_cents":5000},{"up_to_cents":1000}]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("descending bands = %d, want 422", resp.StatusCode)
	}

	createLevel(t, srv.URL, code)

	// unknown level
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/levels/2026/nope", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown level = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/notices", "alice",
		`{"year":2026,"level_code":"nope","claim_reference":"F-1","outstanding_cents":100}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("notice on unknown level = %d, want 404", resp.StatusCode)
	}
	// non-integer year
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/levels/abc/"+code, "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad year = %d, want 400", resp.StatusCode)
	}
	// zero outstanding amount
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/notices", "alice",
		`{"year":2026,"level_code":"`+code+`","claim_reference":"F-1","outstanding_cents":0}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("zero amount = %d, want 422", resp.StatusCode)
	}
	// blank claim reference
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/notices", "alice",
		`{"year":2026,"level_code":"`+code+`","claim_reference":"  ","outstanding_cents":100}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank claim = %d, want 422", resp.StatusCode)
	}
	// duplicate level -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/levels", "alice", levelPayload(code)); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate level = %d, want 409", resp.StatusCode)
	}
}
