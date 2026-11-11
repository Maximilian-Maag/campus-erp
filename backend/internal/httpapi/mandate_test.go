package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// errBoom is an unexpected error for the transport's default branch.
var errBoom = errors.New("boom")

// stubMandates satisfies the handler's mandate interface without a database.
type stubMandates struct {
	m    kasse.Mandate
	all  []kasse.Mandate
	err  error
	seen *mandateFilter
}

// mandateFilter records the filter a list call was given, so a handler test can
// assert the server parsed the query rather than the service guessing.
type mandateFilter struct {
	activeOnly bool
	seq        kasse.MandateSequence
}

func (s stubMandates) CreateMandate(_ context.Context, m kasse.Mandate, _ string) (kasse.Mandate, error) {
	if s.err != nil {
		return kasse.Mandate{}, s.err
	}
	m.ID = 1
	return m, nil
}

func (s stubMandates) Mandate(context.Context, string) (kasse.Mandate, error) {
	if s.err != nil {
		return kasse.Mandate{}, s.err
	}
	return s.m, nil
}

func (s stubMandates) Mandates(_ context.Context, activeOnly bool, seq kasse.MandateSequence) ([]kasse.Mandate, error) {
	if s.seen != nil {
		*s.seen = mandateFilter{activeOnly: activeOnly, seq: seq}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.all != nil {
		return s.all, nil
	}
	return []kasse.Mandate{}, nil
}

func (s stubMandates) RetireMandate(context.Context, string, string) (kasse.Mandate, error) {
	if s.err != nil {
		return kasse.Mandate{}, s.err
	}
	m := s.m
	m.Active = false
	return m, nil
}

const validMandateBody = `{"reference":"MAN-1","debtor_name":"Max Mustermann",` +
	`"debtor_iban":"DE89370400440532013000","debtor_bic":"COBADEFFXXX",` +
	`"signature_date":"2025-03-14","sequence_type":"RCUR"}`

func TestCreateMandateHandler(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"created", validMandateBody, nil, http.StatusCreated},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad date", strings.Replace(validMandateBody, "2025-03-14", "14.03.2025", 1), nil, http.StatusBadRequest},
		{"bad iban", strings.Replace(validMandateBody, "DE89370400440532013000", "DE00", 1), nil, http.StatusUnprocessableEntity},
		{"bad sequence", strings.Replace(validMandateBody, "RCUR", "X", 1), nil, http.StatusUnprocessableEntity},
		{"duplicate", validMandateBody, kasse.ErrMandateReferenceExists, http.StatusConflict},
		{"unexpected error", validMandateBody, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{Mandates: stubMandates{err: c.err}})
			req := httptest.NewRequest(http.MethodPost, "/api/direct-debit/mandates", strings.NewReader(c.body))
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestGetMandateHandler(t *testing.T) {
	srv := httpapi.New(httpapi.Deps{Mandates: stubMandates{m: kasse.Mandate{Reference: "MAN-1"}}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/direct-debit/mandates/MAN-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	missing := httpapi.New(httpapi.Deps{Mandates: stubMandates{err: kasse.ErrMandateNotFound}})
	rec = httptest.NewRecorder()
	missing.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/direct-debit/mandates/NOPE", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", rec.Code)
	}
}

func TestRetireMandateHandler(t *testing.T) {
	cases := []struct {
		name string
		m    kasse.Mandate
		err  error
		want int
	}{
		{"retired", kasse.Mandate{Reference: "MAN-1", Active: false}, nil, http.StatusOK},
		{"unknown reference", kasse.Mandate{}, kasse.ErrMandateNotFound, http.StatusNotFound},
		{"already retired", kasse.Mandate{}, kasse.ErrMandateRetired, http.StatusConflict},
		{"unexpected error", kasse.Mandate{}, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{Mandates: stubMandates{m: c.m, err: c.err}})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec,
				httptest.NewRequest(http.MethodPost, "/api/direct-debit/mandates/MAN-1/retire", nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.want == http.StatusOK && !strings.Contains(rec.Body.String(), `"Active":false`) {
				t.Fatalf("body = %s, want the mandate as it now stands", rec.Body.String())
			}
		})
	}
}

func TestListMandatesHandler(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		err        error
		want       int
		wantActive bool
		wantSeq    kasse.MandateSequence
	}{
		{"no filter", "", nil, http.StatusOK, false, ""},
		{"active and sequence", "?active=true&sequence_type=RCUR", nil, http.StatusOK, true, kasse.SequenceRecurring},
		{"inactive", "?active=false", nil, http.StatusOK, false, ""},
		{"bad active", "?active=maybe", nil, http.StatusUnprocessableEntity, false, ""},
		{"bad sequence", "?sequence_type=XX", nil, http.StatusUnprocessableEntity, false, ""},
		{"unexpected error", "", errBoom, http.StatusInternalServerError, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seen mandateFilter
			stub := stubMandates{
				all:  []kasse.Mandate{{Reference: "MAN-1"}, {Reference: "MAN-2"}},
				err:  c.err,
				seen: &seen,
			}
			srv := httpapi.New(httpapi.Deps{Mandates: stub})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/direct-debit/mandates"+c.query, nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.want != http.StatusOK {
				return
			}
			// The server parsed the query into the service's filter, not the screen's.
			if seen.activeOnly != c.wantActive || seen.seq != c.wantSeq {
				t.Fatalf("filter = {%t %q}, want {%t %q}", seen.activeOnly, seen.seq, c.wantActive, c.wantSeq)
			}
			if body := rec.Body.String(); !strings.Contains(body, "MAN-1") || !strings.HasPrefix(strings.TrimSpace(body), "[") {
				t.Fatalf("body = %s, want a JSON array of mandates", body)
			}
		})
	}
}
