package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// stubETL satisfies the handler's ETL interface. historyCall, when set, records what the
// run-history handler asked for (a value receiver copies the struct, so the record rides on
// a shared pointer).
type stubETL struct {
	names  []string
	report etl.Report
	runs   []etl.Run
	err    error

	historyCall *historyCall
}

type historyCall struct {
	dataset string
	limit   int
}

func (s stubETL) Names() []string { return s.names }

func (s stubETL) Run(context.Context, string, string) (etl.Report, error) {
	if s.err != nil {
		return etl.Report{}, s.err
	}
	return s.report, nil
}

func (s stubETL) History(_ context.Context, name string, limit int) ([]etl.Run, error) {
	if s.historyCall != nil {
		s.historyCall.dataset, s.historyCall.limit = name, limit
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.runs, nil
}

func TestListETLDatasetsHandler(t *testing.T) {
	srv := httpapi.New(httpapi.Deps{ETL: stubETL{names: []string{"lookup/bank-leit-zahl"}}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/datasets", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "lookup/bank-leit-zahl") {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
}

// TestListETLRunsHandler: the run history is served, the dataset and limit reach the seam,
// and a limit that is not a positive integer is refused rather than passed on.
func TestListETLRunsHandler(t *testing.T) {
	call := &historyCall{}
	runs := []etl.Run{{Actor: "alice", Summary: "source=2 loaded=2 checksum=ab"}}
	srv := httpapi.New(httpapi.Deps{ETL: stubETL{runs: runs, historyCall: call}})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/runs?dataset=lookup/x&limit=5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "alice") || !strings.Contains(rec.Body.String(), "checksum=ab") {
		t.Fatalf("body = %q, want the runs", rec.Body.String())
	}
	if call.dataset != "lookup/x" || call.limit != 5 {
		t.Fatalf("history got (%q, %d), want (lookup/x, 5)", call.dataset, call.limit)
	}

	// No limit means the seam decides (0 -> the audit store's default).
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/runs", nil))
	if rec.Code != http.StatusOK || call.dataset != "" || call.limit != 0 {
		t.Fatalf("status = %d, history got (%q, %d); want 200 with (\"\", 0)", rec.Code, call.dataset, call.limit)
	}

	for _, q := range []string{"limit=0", "limit=-2", "limit=abc"} {
		rec = httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/runs?"+q, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status = %d, want 422", q, rec.Code)
		}
		var problem httpapi.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != "invalid_limit" {
			t.Fatalf("%s: code = %q (%v), want invalid_limit", q, problem.Code, err)
		}
	}
}

func TestListETLRunsHandlerStoreFailure(t *testing.T) {
	srv := httpapi.New(httpapi.Deps{ETL: stubETL{err: errBoom}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/runs", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestRunETLDatasetHandler(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"ok", `{"dataset":"lookup/bank-leit-zahl"}`, nil, http.StatusOK},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"unknown", `{"dataset":"nope"}`, etl.ErrUnknownDataset, http.StatusNotFound},
		{"reconcile", `{"dataset":"x"}`, etl.ErrReconcile, http.StatusUnprocessableEntity},
		{"unexpected", `{"dataset":"x"}`, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{ETL: stubETL{report: etl.Report{Dataset: "x", Source: 2, Loaded: 2}, err: c.err}})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/etl/runs", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}
