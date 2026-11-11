package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// The handler path carries the code to the client: a refusal the service returns as a domain error
// reaches the envelope with that error's code, not with the status's.
func TestHandlerAnswersWithTheDomainCode(t *testing.T) {
	srv := httpapi.New(httpapi.Deps{CollectionRuns: stubCollectionRuns{err: kasse.ErrMandateNotActive}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/direct-debit/runs",
		strings.NewReader(validCollectionBody)))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (%s)", rec.Code, rec.Body.String())
	}
	var p httpapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Code != "kasse.mandate_not_active" {
		t.Fatalf("code = %q, want kasse.mandate_not_active", p.Code)
	}
}
