package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// stubReturns records the period it was asked for and returns a canned set of lines.
type stubReturns struct {
	lines    []kasse.StatementLine
	err      error
	seenFrom time.Time
	seenTo   time.Time
	calls    int
}

func (s *stubReturns) Returns(_ context.Context, from, to time.Time) ([]kasse.StatementLine, error) {
	s.calls++
	s.seenFrom, s.seenTo = from, to
	return s.lines, s.err
}

func TestListReturnedLinesHandler(t *testing.T) {
	stub := &stubReturns{lines: []kasse.StatementLine{{
		ID: 7, ValueDate: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), Credit: false, Return: true,
		AmountCents: 12500, Currency: "EUR", PostingKey: "009", ClientRef: "ANORD-2026-9",
		Purpose: "Rückläufer", Status: kasse.StatusReturned,
	}}}
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{ReturnPostings: stub}).Routes())
	defer srv.Close()

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statement-returns?from=2026-11-01&to=2026-11-30", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", resp.StatusCode, readBody(t, resp))
	}
	if stub.calls != 1 {
		t.Fatalf("service called %d times", stub.calls)
	}
	if got := stub.seenFrom.Format(time.DateOnly); got != "2026-11-01" {
		t.Fatalf("from = %q", got)
	}
	if got := stub.seenTo.Format(time.DateOnly); got != "2026-11-30" {
		t.Fatalf("to = %q", got)
	}
	var got []struct {
		ID         int64
		PostingKey string
		ClientRef  string
		Return     bool
		Status     string
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != 7 || got[0].PostingKey != "009" ||
		got[0].ClientRef != "ANORD-2026-9" || !got[0].Return || got[0].Status != kasse.StatusReturned {
		t.Fatalf("lines = %+v", got)
	}
}

func TestListReturnedLinesHandlerErrors(t *testing.T) {
	cases := []struct {
		name  string
		deps  httpapi.Deps
		query string
		want  int
	}{
		{"unconfigured", httpapi.Deps{}, "?from=2026-11-01&to=2026-11-30", http.StatusServiceUnavailable},
		{"missing from", httpapi.Deps{ReturnPostings: &stubReturns{}}, "?to=2026-11-30", http.StatusUnprocessableEntity},
		{"missing to", httpapi.Deps{ReturnPostings: &stubReturns{}}, "?from=2026-11-01", http.StatusUnprocessableEntity},
		{"bad from", httpapi.Deps{ReturnPostings: &stubReturns{}}, "?from=01.11.2026&to=2026-11-30", http.StatusUnprocessableEntity},
		{"inverted period", httpapi.Deps{ReturnPostings: &stubReturns{err: kasse.ErrReturnsPeriodInvalid}}, "?from=2026-11-30&to=2026-11-01", http.StatusUnprocessableEntity},
		{"unexpected", httpapi.Deps{ReturnPostings: &stubReturns{err: errBoom}}, "?from=2026-11-01&to=2026-11-30", http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(httpapi.New(c.deps).Routes())
			defer srv.Close()
			resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statement-returns"+c.query, "carol", "")
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, c.want, readBody(t, resp))
			}
		})
	}
}

func TestRequestReturnPostingsHandler(t *testing.T) {
	queue := &stubExports{id: 42}
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Exports: queue}).Routes())
	defer srv.Close()

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/ledger/return-postings", "carol",
		`{"from":"2026-11-01","to":"2026-11-30"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", resp.StatusCode, readBody(t, resp))
	}
	var got struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.JobID != 42 {
		t.Fatalf("job id = %d, want 42", got.JobID)
	}
	want := []string{"returns", "2026-11-01", "2026-11-30", "carol"}
	if queue.calls != 1 || len(queue.args) != len(want) {
		t.Fatalf("queue calls = %d, args = %v", queue.calls, queue.args)
	}
	for i, w := range want {
		if queue.args[i] != w {
			t.Fatalf("queue args[%d] = %q, want %q", i, queue.args[i], w)
		}
	}
}

func TestRequestReturnPostingsHandlerErrors(t *testing.T) {
	cases := []struct {
		name string
		deps httpapi.Deps
		body string
		want int
	}{
		{"unconfigured", httpapi.Deps{}, `{"from":"2026-11-01","to":"2026-11-30"}`, http.StatusServiceUnavailable},
		{"bad json", httpapi.Deps{Exports: &stubExports{}}, `{`, http.StatusBadRequest},
		{"bad from", httpapi.Deps{Exports: &stubExports{}}, `{"from":"01.11.2026","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"bad to", httpapi.Deps{Exports: &stubExports{}}, `{"from":"2026-11-01","to":""}`, http.StatusUnprocessableEntity},
		{"queue fails", httpapi.Deps{Exports: &stubExports{err: errBoom}}, `{"from":"2026-11-01","to":"2026-11-30"}`, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(httpapi.New(c.deps).Routes())
			defer srv.Close()
			resp := doJSON(t, http.MethodPost, srv.URL+"/api/ledger/return-postings", "carol", c.body)
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, c.want, readBody(t, resp))
			}
		})
	}
}

// TestHTTPReturnPostingsEndToEnd drives the read through the real service and database: a
// paid order whose statement line came back as a return is listed for the period it is
// effective in, with the posting key the bank sent.
func TestHTTPReturnPostingsEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	// A debtor account and one paid order (create -> submit -> approve -> run).
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/cash-accounts", "carol",
		`{"name":"Universität","iban":"DE02120300000000202051","bic":"BYLADEM1001","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create account = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var account accountJSON
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	order := createApprovedOrder(t, srv.URL, 4999)
	runBody := `{"reference":"RUN-` + uniqueSuffix(t) + `","account_id":` + itoa(account.ID) +
		`,"execution_date":"2026-11-18","order_ids":[` + itoa(order.ID) + `]}`
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "carol", runBody); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run = %d", resp.StatusCode)
	}

	// The bank sends the movement back: posting key 059 is a payment-out return.
	const day = "2026-11-20"
	content := ":20:STMT-" + uniqueSuffix(t) + "\n" +
		":25:DE02120300000000202051\n:28C:00001/001\n:60F:C261120EUR10000,00\n" +
		":61:2611201120C49,99N059" + order.Reference + "//BANKREF\n" +
		":86:Rückläufer\n" +
		":62F:C261120EUR10000,00\n-\n"
	importStatement(t, srv.URL, account.ID, content)

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/bank-statement-returns?from="+day+"&to="+day, "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list returns = %d (%s)", resp.StatusCode, readBody(t, resp))
	}
	var lines []struct {
		ID          int64
		ClientRef   string
		PostingKey  string
		Credit      bool
		Return      bool
		Status      string
		AmountCents int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&lines); err != nil {
		t.Fatalf("decode lines: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("no returning line listed for %s", day)
	}
	for _, l := range lines {
		if l.ClientRef != order.Reference {
			continue
		}
		if !l.Return || l.Status != kasse.StatusReturned || l.PostingKey != "059" ||
			!l.Credit || l.AmountCents != 4999 {
			t.Fatalf("returned line = %+v", l)
		}
		return
	}
	t.Fatalf("order %s is not among the %d returning lines", order.Reference, len(lines))
}

// A period with no returning line is an empty list, not an error — and a period that is
// not a period is refused, so the two never look alike.
func TestHTTPReturnPostingsEmptyPeriod(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statement-returns?from=3000-01-01&to=3000-01-02", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", resp.StatusCode, readBody(t, resp))
	}
	var lines []any
	if err := json.NewDecoder(resp.Body).Decode(&lines); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("lines = %v, want none", lines)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statement-returns?from=not-a-date&to=3000-01-02", "carol", ""); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad period = %d, want 422", resp.StatusCode)
	}
}
