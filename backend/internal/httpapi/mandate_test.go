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
	m   kasse.Mandate
	err error
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
