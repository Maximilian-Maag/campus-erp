package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

// TestHTTPBudgetSourceAndCenter drives the whole path through the real service and database: a
// Mittelquelle and a Bewirtschaftungsstelle are created and listed, a position is assigned both,
// and the assignment is readable back — the position's own read carries it.
func TestHTTPBudgetSourceAndCenter(t *testing.T) {
	srv, _ := newTestServer(t)

	sourceCode := "LHH-" + uniqueSuffix(t)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/sources", "alice",
		`{"code":"`+sourceCode+`","name":"Landesmittel","chapter":"05"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create source = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var source struct {
		ID      int64
		Code    string
		Name    string
		Chapter string
		Active  bool
	}
	if err := json.NewDecoder(resp.Body).Decode(&source); err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if source.ID == 0 || source.Code != sourceCode || source.Name != "Landesmittel" ||
		source.Chapter != "05" || !source.Active {
		t.Fatalf("source = %+v", source)
	}

	centerCode := "BW-" + uniqueSuffix(t)
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/budget/centers", "alice",
		`{"code":"`+centerCode+`","name":"Fakultät 1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create centre = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var center struct {
		ID   int64
		Code string
		Name string
	}
	if err := json.NewDecoder(resp.Body).Decode(&center); err != nil {
		t.Fatalf("decode centre: %v", err)
	}
	if center.ID == 0 || center.Code != centerCode {
		t.Fatalf("centre = %+v", center)
	}

	// Both lists carry what was created.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/sources", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list sources = %d", resp.StatusCode)
	}
	var sources []struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&sources); err != nil {
		t.Fatalf("decode sources: %v", err)
	}
	var sawSource bool
	for _, s := range sources {
		if s.ID == source.ID {
			sawSource = true
		}
	}
	if !sawSource {
		t.Fatalf("the created source is not among %d sources", len(sources))
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/centers", "alice", "")
	var centers []struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&centers); err != nil {
		t.Fatalf("decode centres: %v", err)
	}
	var sawCenter bool
	for _, c := range centers {
		if c.ID == center.ID {
			sawCenter = true
		}
	}
	if !sawCenter {
		t.Fatalf("the created centre is not among %d centres", len(centers))
	}

	// A position is assigned both halves, and the assignment comes back on its own read.
	positionID := newBudgetPosition(t, srv.URL)
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(positionID)+"/assignment", "alice",
		fmt.Sprintf(`{"source_id":%d,"center_id":%d}`, source.ID, center.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assign = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var assigned struct {
		ID       int64
		SourceID *int64
		CenterID *int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&assigned); err != nil {
		t.Fatalf("decode assigned: %v", err)
	}
	if assigned.SourceID == nil || *assigned.SourceID != source.ID || assigned.CenterID == nil || *assigned.CenterID != center.ID {
		t.Fatalf("assigned = %+v", assigned)
	}
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/"+itoa(positionID), "alice", "")
	var loaded struct {
		Position struct {
			ID       int64
			SourceID *int64
			CenterID *int64
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode position: %v", err)
	}
	if loaded.Position.SourceID == nil || *loaded.Position.SourceID != source.ID ||
		loaded.Position.CenterID == nil || *loaded.Position.CenterID != center.ID {
		t.Fatalf("loaded position = %+v", loaded.Position)
	}

	// Clearing one half leaves the other, which is what a nullable half is for.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(positionID)+"/assignment", "alice",
		fmt.Sprintf(`{"source_id":null,"center_id":%d}`, center.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear the source = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var cleared struct {
		SourceID *int64
		CenterID *int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared: %v", err)
	}
	if cleared.SourceID != nil || cleared.CenterID == nil {
		t.Fatalf("cleared = %+v", cleared)
	}
}

// TestHTTPBudgetSourceRefusals covers the refusals at the edge: what the transport refuses, and what
// the domain refuses, each with its own status.
func TestHTTPBudgetSourceRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	positionID := newBudgetPosition(t, srv.URL)

	code := "DUP-" + uniqueSuffix(t)
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/sources", "alice",
		`{"code":"`+code+`","name":"Erste"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first source = %d", resp.StatusCode)
	}
	// The same code again is a conflict, not a second source.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/sources", "alice",
		`{"code":"`+code+`","name":"Zweite"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate source = %d, want 409", resp.StatusCode)
	}
	// A body that is not JSON, a source without a code, and a centre without a name.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/sources", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/sources", "alice",
		`{"code":"  ","name":"Ohne Code"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank code = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/centers", "alice",
		`{"code":"X-`+uniqueSuffix(t)+`","name":"  "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank name = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/centers", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad centre body = %d, want 400", resp.StatusCode)
	}

	// An assignment naming a source nothing carries, an unknown position, and a path that is not
	// an id.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(positionID)+"/assignment", "alice",
		`{"source_id":9223372036854775807}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown source = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/9223372036854775807/assignment", "alice",
		`{}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown position = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/abc/assignment", "alice",
		`{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/positions/"+itoa(positionID)+"/assignment", "alice",
		`{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad assignment body = %d, want 400", resp.StatusCode)
	}

	// The refused assignment left the position unassigned.
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/budget/positions/"+itoa(positionID), "alice", "")
	var loaded struct {
		Position struct {
			SourceID *int64
			CenterID *int64
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode position: %v", err)
	}
	if loaded.Position.SourceID != nil || loaded.Position.CenterID != nil {
		t.Fatalf("a refused assignment changed the position: %+v", loaded.Position)
	}
}

// A budget that carries neither reports empty lists rather than failing: the two are
// configuration, and a fresh install has none.
func TestHTTPBudgetSourceEmptyLists(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Budget: stubBudget{}}).Routes())
	t.Cleanup(srv.Close)
	for _, path := range []string{"/api/budget/sources", "/api/budget/centers"} {
		resp := doJSON(t, http.MethodGet, srv.URL+path, "alice", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s = %d, want 200", path, resp.StatusCode)
		}
	}
}
