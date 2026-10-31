package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// stubETL satisfies the handler's ETL interface.
type stubETL struct {
	names  []string
	report etl.Report
	err    error
}

func (s stubETL) Names() []string { return s.names }

func (s stubETL) Run(context.Context, string, string) (etl.Report, error) {
	if s.err != nil {
		return etl.Report{}, s.err
	}
	return s.report, nil
}

func TestListETLDatasetsHandler(t *testing.T) {
	srv := httpapi.New(httpapi.Deps{ETL: stubETL{names: []string{"lookup/bank-leit-zahl"}}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/etl/datasets", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "lookup/bank-leit-zahl") {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
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
