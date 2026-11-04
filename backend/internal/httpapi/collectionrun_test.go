package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// stubCollectionRuns satisfies the handler's collection-run interface.
type stubCollectionRuns struct {
	run  kasse.CollectionRun
	file string
	err  error
}

func (s stubCollectionRuns) CreateCollectionRun(_ context.Context, reference string, _ int64, _ string,
	_ time.Time, _ kasse.MandateSequence, _ string, _ []kasse.CollectionItem, _ string) (kasse.CollectionRun, error) {
	if s.err != nil {
		return kasse.CollectionRun{}, s.err
	}
	s.run.Reference = reference
	return s.run, nil
}

func (s stubCollectionRuns) CreateCollectionRunFromReceivables(_ context.Context, reference string, _ int64,
	_ string, _ time.Time, _ kasse.MandateSequence, _ string, _ time.Time, _ string) (kasse.CollectionRun, error) {
	if s.err != nil {
		return kasse.CollectionRun{}, s.err
	}
	s.run.Reference = reference
	return s.run, nil
}

func (s stubCollectionRuns) CollectionRun(context.Context, string) (kasse.CollectionRun, error) {
	return s.run, s.err
}

func (s stubCollectionRuns) CollectionRunFile(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.file, nil
}

const validCollectionBody = `{"reference":"RUN-1","account_id":1,"creditor_id":"DE98ZZZ09999999999",` +
	`"collection_date":"2026-11-01","sequence_type":"RCUR","local_instrument":"CORE",` +
	`"items":[{"mandate_reference":"MAN-1","end_to_end_id":"E1","amount_cents":1000,"purpose":"x"}]}`

func TestCreateCollectionRunHandler(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"created", validCollectionBody, nil, http.StatusCreated},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad date", strings.Replace(validCollectionBody, "2026-11-01", "01.11.2026", 1), nil, http.StatusBadRequest},
		{"validation", validCollectionBody, kasse.ErrNoCollectionItems, http.StatusUnprocessableEntity},
		{"duplicate", validCollectionBody, kasse.ErrRunReferenceExists, http.StatusConflict},
		{"unexpected", validCollectionBody, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{CollectionRuns: stubCollectionRuns{err: c.err}})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/direct-debit/runs", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

const validFromReceivablesBody = `{"reference":"RUN-2","account_id":1,"creditor_id":"DE98ZZZ09999999999",` +
	`"collection_date":"2026-11-01","sequence_type":"RCUR","local_instrument":"CORE","due_by":"2026-10-15"}`

func TestCreateCollectionRunFromReceivablesHandler(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"created", validFromReceivablesBody, nil, http.StatusCreated},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad collection date", strings.Replace(validFromReceivablesBody, "2026-11-01", "01.11.2026", 1), nil, http.StatusBadRequest},
		{"bad due by", strings.Replace(validFromReceivablesBody, "2026-10-15", "15.10.2026", 1), nil, http.StatusBadRequest},
		{"nothing due", validFromReceivablesBody, kasse.ErrNoCollectionItems, http.StatusUnprocessableEntity},
		{"due after collection", validFromReceivablesBody, kasse.ErrReceivableDueByAfterCollection, http.StatusUnprocessableEntity},
		{"currency mismatch", validFromReceivablesBody, kasse.ErrReceivableCurrencyMismatch, http.StatusUnprocessableEntity},
		{"unexpected", validFromReceivablesBody, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{CollectionRuns: stubCollectionRuns{err: c.err}})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/direct-debit/runs/from-receivables", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestGetCollectionRunFileHandler(t *testing.T) {
	ok := httpapi.New(httpapi.Deps{CollectionRuns: stubCollectionRuns{file: "<Document/>"}})
	rec := httptest.NewRecorder()
	ok.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/direct-debit/runs/RUN-1/file", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<Document/>") {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("content-type = %q", ct)
	}

	missing := httpapi.New(httpapi.Deps{CollectionRuns: stubCollectionRuns{err: kasse.ErrCollectionRunNotFound}})
	rec = httptest.NewRecorder()
	missing.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/direct-debit/runs/NOPE/file", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", rec.Code)
	}
}
