package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// TestHTTPBudgetImport drives the whole path through the real service: one file creates a source, a
// centre and a position, names the position from its own line, and imports the same file a second
// time as a no-op counted as kept.
func TestHTTPBudgetImport(t *testing.T) {
	srv, _ := newTestServer(t)
	code := fmt.Sprintf("IMP-%d", time.Now().UnixNano())
	file := strings.Join([]string{
		"source_code;source_name;source_chapter;center_code;center_name;year;chapter;title;description;ansatz",
		code + ";Landesmittel;05;" + code + "-BW;Fakultät 1;2026;05;" + code + ";Beschaffung;10.000,00",
		"",
	}, "\n")
	body := fmt.Sprintf(`{"content":%q}`, file)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/imports", "carol", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var res budget.ImportResult
	decodeInto(t, resp, &res)
	if res.Layout != "budget-import/v1" {
		t.Fatalf("layout = %q, want the standard layout", res.Layout)
	}
	if res.SourcesCreated != 1 || res.CentersCreated != 1 || res.PositionsCreated != 1 || res.PositionsNamed != 1 {
		t.Fatalf("import = %+v, want one of each created and the position named", res)
	}

	// The file is what the API now serves: the source, and the position named by it.
	sources := listBudgetSources(t, srv.URL)
	found := false
	for _, s := range sources {
		if s.Code == code {
			found = true
			if s.Name != "Landesmittel" || s.Chapter != "05" {
				t.Fatalf("source = %+v, want the file's name and chapter", s)
			}
		}
	}
	if !found {
		t.Fatalf("the imported source %s is missing from the list", code)
	}
	position := findHTTPPosition(t, srv.URL, 2026, "05", code)
	if position.SourceID == nil || position.CenterID == nil {
		t.Fatalf("position = %+v, want it named by the file", position)
	}

	// The same file again: everything kept, nothing created, nothing named twice.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/budget/imports", "carol", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second status = %d, want 201", resp.StatusCode)
	}
	var again budget.ImportResult
	decodeInto(t, resp, &again)
	if again.SourcesCreated != 0 || again.PositionsCreated != 0 || again.PositionsNamed != 0 ||
		again.SourcesKept != 1 || again.PositionsKept != 1 {
		t.Fatalf("second import = %+v, want everything kept", again)
	}
}

// A refused import is refused whole: the route answers 422 and names the line, and writes nothing.
func TestHTTPBudgetImportRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	code := fmt.Sprintf("BAD-%d", time.Now().UnixNano())
	header := "source_code;source_name;source_chapter;center_code;center_name;year;chapter;title;description;ansatz\n"
	good := code + ";Mittel;05;" + code + "-BW;Stelle;2026;05;" + code + ";x;1,00\n"
	bad := code + "-1;Mittel;05;" + code + "-BW;Stelle;2026;05;" + code + "-1;x;zuviel\n"

	cases := []struct {
		name   string
		body   string
		status int
		want   string
	}{
		{"a bad row", fmt.Sprintf(`{"content":%q}`, header+good+bad), http.StatusUnprocessableEntity, "row 3"},
		{"an empty file", `{"content":"  \n"}`, http.StatusUnprocessableEntity, "nothing to import"},
		{"an unknown layout", fmt.Sprintf(`{"layout":"vendor/v9","content":%q}`, header+good), http.StatusUnprocessableEntity, "unknown layout"},
		{"broken json", `{"content":`, http.StatusBadRequest, "bad request"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, srv.URL+"/api/budget/imports", "carol", c.body)
			if resp.StatusCode != c.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.status)
			}
			if body := readBody(t, resp); !strings.Contains(body, c.want) {
				t.Fatalf("body = %q, want it to mention %q", body, c.want)
			}
		})
	}

	// Nothing of the refused files was written, not even the readable rows.
	for _, s := range listBudgetSources(t, srv.URL) {
		if strings.HasPrefix(s.Code, code) {
			t.Fatalf("source %s was written although the import was refused", s.Code)
		}
	}
	if _, found := findHTTPPositionOK(t, srv.URL, 2026, "05", code); found {
		t.Fatal("a position was written although the import was refused")
	}
}

func listBudgetSources(t *testing.T, base string) []budget.Source {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/budget/sources", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list sources: status = %d", resp.StatusCode)
	}
	var sources []budget.Source
	decodeInto(t, resp, &sources)
	return sources
}

func findHTTPPosition(t *testing.T, base string, year int, chapter, title string) budget.Position {
	t.Helper()
	pos, found := findHTTPPositionOK(t, base, year, chapter, title)
	if !found {
		t.Fatalf("position %d/%s/%s is missing", year, chapter, title)
	}
	return pos
}

func findHTTPPositionOK(t *testing.T, base string, year int, chapter, title string) (budget.Position, bool) {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/budget/positions", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list positions: status = %d", resp.StatusCode)
	}
	var rows []struct {
		Position budget.Position `json:"position"`
	}
	decodeInto(t, resp, &rows)
	for _, r := range rows {
		if r.Position.Year == year && r.Position.Chapter == chapter && r.Position.Title == title {
			return r.Position, true
		}
	}
	return budget.Position{}, false
}

// decodeInto reads a JSON response body into v, failing the test if the body is not JSON.
func decodeInto(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode the response: %v", err)
	}
}
