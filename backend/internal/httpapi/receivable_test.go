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

// stubReceivables satisfies the handler's open-items interface.
type stubReceivables struct {
	items []kasse.Receivable
	one   kasse.Receivable
	err   error
}

func (s stubReceivables) Post(_ context.Context, mandateReference string, amountCents int64,
	currency string, _ time.Time, _, _, _ string) (kasse.Receivable, error) {
	if s.err != nil {
		return kasse.Receivable{}, s.err
	}
	r := s.one
	r.MandateReference = mandateReference
	r.AmountCents = amountCents
	r.Currency = currency
	return r, nil
}

func (s stubReceivables) Receivable(context.Context, int64) (kasse.Receivable, error) {
	return s.one, s.err
}

func (s stubReceivables) Receivables(context.Context, kasse.ReceivableState) ([]kasse.Receivable, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.items, nil
}

const validReceivableBody = `{"mandate_reference":"MAN-1","amount_cents":4250,"currency":"EUR",` +
	`"due_date":"2026-10-15","purpose":"Semesterbeitrag","source":"manual"}`

func TestCreateReceivableHandler(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"created", validReceivableBody, nil, http.StatusCreated},
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad date", strings.Replace(validReceivableBody, "2026-10-15", "15.10.2026", 1), nil, http.StatusBadRequest},
		{"validation", validReceivableBody, kasse.ErrReceivableAmountNotPositive, http.StatusUnprocessableEntity},
		{"unknown mandate", validReceivableBody, kasse.ErrMandateNotFound, http.StatusNotFound},
		{"unexpected", validReceivableBody, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httpapi.New(httpapi.Deps{Receivables: stubReceivables{err: c.err}})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/receivables", strings.NewReader(c.body)))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestListReceivablesHandler(t *testing.T) {
	items := []kasse.Receivable{{ID: 1, MandateReference: "MAN-1", AmountCents: 100, State: kasse.ReceivableOpen}}
	ok := httpapi.New(httpapi.Deps{Receivables: stubReceivables{items: items}})
	rec := httptest.NewRecorder()
	ok.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables?state=open", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "MAN-1") {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}

	// An unknown state is refused before the store is touched.
	bad := httpapi.New(httpapi.Deps{Receivables: stubReceivables{items: items}})
	rec = httptest.NewRecorder()
	bad.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables?state=paid", nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad state = %d, want 422", rec.Code)
	}

	fail := httpapi.New(httpapi.Deps{Receivables: stubReceivables{err: errBoom}})
	rec = httptest.NewRecorder()
	fail.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failing list = %d, want 500", rec.Code)
	}
}

func TestGetReceivableHandler(t *testing.T) {
	one := kasse.Receivable{ID: 7, MandateReference: "MAN-1", AmountCents: 100}
	ok := httpapi.New(httpapi.Deps{Receivables: stubReceivables{one: one}})
	rec := httptest.NewRecorder()
	ok.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables/7", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "MAN-1") {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}

	bad := httpapi.New(httpapi.Deps{Receivables: stubReceivables{one: one}})
	rec = httptest.NewRecorder()
	bad.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables/nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-numeric id = %d, want 400", rec.Code)
	}

	missing := httpapi.New(httpapi.Deps{Receivables: stubReceivables{err: kasse.ErrReceivableNotFound}})
	rec = httptest.NewRecorder()
	missing.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/receivables/9", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d, want 404", rec.Code)
	}
}
