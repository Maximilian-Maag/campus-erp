package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// stubDunningRun records the run's inputs and returns a canned report.
type stubDunningRun struct {
	result   kasse.DunningRunResult
	err      error
	seenYear int
	seenAsOf time.Time
}

func (s *stubDunningRun) Run(_ context.Context, year int, asOf time.Time, _ string) (kasse.DunningRunResult, error) {
	s.seenYear = year
	s.seenAsOf = asOf
	return s.result, s.err
}

func TestRunDunningHandler(t *testing.T) {
	stub := &stubDunningRun{result: kasse.DunningRunResult{
		Year: 2026, AsOf: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Issued: 1, Outcomes: []kasse.DunningRunOutcome{{ReceivableID: 7, ClaimReference: "receivable/7", Issued: true}},
	}}
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{DunningRun: stub}).Handler())
	defer srv.Close()

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/runs", "alice", `{"year":2026,"as_of":"2026-01-15"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", resp.StatusCode, readBody(t, resp))
	}
	if stub.seenYear != 2026 {
		t.Fatalf("year = %d, want 2026", stub.seenYear)
	}
	if got := stub.seenAsOf.Format(time.DateOnly); got != "2026-01-15" {
		t.Fatalf("as-of = %q, want 2026-01-15", got)
	}
	var got struct {
		Issued   int
		Outcomes []struct {
			ClaimReference string
			Issued         bool
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Issued != 1 || len(got.Outcomes) != 1 || !got.Outcomes[0].Issued ||
		got.Outcomes[0].ClaimReference != "receivable/7" {
		t.Fatalf("report = %+v", got)
	}
}

func TestRunDunningHandlerErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"bad date", `{"year":2026,"as_of":"15.01.2026"}`, nil, http.StatusBadRequest},
		{"bad year", `{"year":1800,"as_of":"2026-01-15"}`, kasse.ErrRunYearInvalid, http.StatusUnprocessableEntity},
		{"no date", `{"year":2026,"as_of":"2026-01-15"}`, kasse.ErrRunDateRequired, http.StatusUnprocessableEntity},
		{"chain head", `{"year":2026,"as_of":"2026-01-15"}`, kasse.ErrDunningChainHead, http.StatusConflict},
		{"chain broken", `{"year":2026,"as_of":"2026-01-15"}`, kasse.ErrDunningChainBroken, http.StatusConflict},
		{"unexpected", `{"year":2026,"as_of":"2026-01-15"}`, errBoom, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(httpapi.New(httpapi.Deps{DunningRun: &stubDunningRun{err: c.err}}).Handler())
			defer srv.Close()
			resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/runs", "alice", c.body)
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, c.want, readBody(t, resp))
			}
		})
	}
}

// TestHTTPDunningRunEndToEnd drives the run through the real service and database:
// two levels, one overdue receivable, one run, then the same run again.
func TestHTTPDunningRunEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)
	year := 2000 + int(testSeqNext(t)%900)
	first := "A" + uniqueSuffix(t)
	second := "B" + uniqueSuffix(t)
	for code, next := range map[string]string{first: second, second: ""} {
		body := fmt.Sprintf(`{"year":%d,"code":%q,"description":"Stufe","days_allowed":0,`+
			`"active":true,"next_code":%q,"bands":[{"up_to_cents":0,"fee_cents":500,"rate_basis_points":0}]}`,
			year, code, next)
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/dunning/levels", "alice", body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create level %s = %d (%s)", code, resp.StatusCode, readBody(t, resp))
		}
	}

	mandateRef := "MAN-" + uniqueSuffix(t)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/direct-debit/mandates", "alice",
		fmt.Sprintf(`{"reference":%q,"debtor_name":"Max Mustermann",`+
			`"debtor_iban":"DE89370400440532013000","debtor_bic":"COBADEFFXXX",`+
			`"signature_date":"2025-03-14","sequence_type":"RCUR"}`, mandateRef))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create mandate = %d (%s)", resp.StatusCode, readBody(t, resp))
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/receivables", "alice",
		fmt.Sprintf(`{"mandate_reference":%q,"amount_cents":10000,"currency":"EUR",`+
			`"due_date":"%d-12-31","purpose":"Semesterbeitrag","source":"manual"}`, mandateRef, year-1))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create receivable = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var rec struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		t.Fatalf("decode receivable: %v", err)
	}
	claim := kasse.ReceivableClaimReference(rec.ID)

	runBody := fmt.Sprintf(`{"year":%d,"as_of":"%d-01-01"}`, year, year)
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/dunning/runs", "alice", runBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var report struct {
		Issued   int
		Skipped  int
		Outcomes []struct {
			ClaimReference string
			LevelCode      string
			ChargeCents    int64
			Issued         bool
			Reason         string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	var found bool
	for _, o := range report.Outcomes {
		if o.ClaimReference != claim {
			continue
		}
		found = true
		if !o.Issued || o.LevelCode != first || o.ChargeCents != 500 {
			t.Fatalf("outcome = %+v", o)
		}
	}
	if !found {
		t.Fatalf("claim %q missing from the report: %+v", claim, report.Outcomes)
	}

	// The same run the same day must skip the claim rather than dun it twice.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/dunning/runs", "alice", runBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second run = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var again struct {
		Outcomes []struct {
			ClaimReference string
			Issued         bool
			Reason         string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&again); err != nil {
		t.Fatalf("decode second run: %v", err)
	}
	for _, o := range again.Outcomes {
		if o.ClaimReference == claim {
			if o.Issued || o.Reason != kasse.DunningSkipAlreadyDunned {
				t.Fatalf("second outcome = %+v, want already_dunned", o)
			}
			return
		}
	}
	t.Fatalf("claim %q missing from the second report", claim)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf := make([]byte, 2048)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}
