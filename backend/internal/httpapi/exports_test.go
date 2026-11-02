package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

// stubExports records the last queued request, prefixed with the operation that made it.
type stubExports struct {
	calls int
	args  []string
	id    int64
	err   error
}

func (s *stubExports) record(args ...string) { s.calls++; s.args = args }

func (s *stubExports) EnqueueStatisticsExport(_ context.Context, state, from, to, actor string) (int64, error) {
	s.record("statistics", state, from, to, actor)
	return s.id, s.err
}

func (s *stubExports) EnqueueLedgerExport(_ context.Context, from, to, actor string) (int64, error) {
	s.record("ledger", from, to, actor)
	return s.id, s.err
}

func (s *stubExports) EnqueueReportRender(_ context.Context, reportID, from, to, actor string) (int64, error) {
	s.record("report", reportID, from, to, actor)
	return s.id, s.err
}

func exportServer(t *testing.T, q *stubExports) *httptest.Server {
	t.Helper()
	deps := httpapi.Deps{}
	if q != nil {
		deps.Exports = q
	}
	srv := httptest.NewServer(httpapi.New(deps).Routes())
	t.Cleanup(srv.Close)
	return srv
}

// TestExportRoutesQueue covers every enqueue route: the caller gets 202 and the queued
// job id, and the seam receives the arguments the request carried.
func TestExportRoutesQueue(t *testing.T) {
	cases := []struct {
		name, path, body string
		want             []string
	}{
		{
			"statistics", "/api/statistics/exports",
			`{"state":" bw ","from":"2026-11-01","to":"2026-11-30"}`,
			[]string{"statistics", "bw", "2026-11-01", "2026-11-30", "carol"},
		},
		{
			"ledger", "/api/ledger/exports",
			`{"from":"2026-11-01","to":"2026-11-30"}`,
			[]string{"ledger", "2026-11-01", "2026-11-30", "carol"},
		},
		{
			"report", "/api/reports/renders",
			`{"report_id":" kasse-daily-close ","from":"2026-11-01","to":"2026-11-30"}`,
			[]string{"report", "kasse-daily-close", "2026-11-01", "2026-11-30", "carol"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &stubExports{id: 7}
			srv := exportServer(t, stub)
			resp := doJSON(t, http.MethodPost, srv.URL+c.path, "carol", c.body)
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", resp.StatusCode)
			}
			var got struct {
				JobID int64 `json:"job_id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.JobID != 7 {
				t.Fatalf("job_id = %d, want 7", got.JobID)
			}
			if stub.calls != 1 || !reflect.DeepEqual(stub.args, c.want) {
				t.Fatalf("queue got %q (%d calls), want %q", stub.args, stub.calls, c.want)
			}
		})
	}
}

// TestExportRoutesRefuseBadRequests: a request the route cannot honour is refused at the
// edge, so a doomed job is never queued.
func TestExportRoutesRefuseBadRequests(t *testing.T) {
	cases := []struct {
		name, path, body string
		want             int
	}{
		{"statistics malformed", "/api/statistics/exports", `{`, http.StatusBadRequest},
		{"statistics no state", "/api/statistics/exports", `{"from":"2026-11-01","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"statistics bad from", "/api/statistics/exports", `{"state":"bw","from":"01.11.2026","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"statistics bad to", "/api/statistics/exports", `{"state":"bw","from":"2026-11-01","to":"nope"}`, http.StatusUnprocessableEntity},
		{"ledger malformed", "/api/ledger/exports", `{`, http.StatusBadRequest},
		{"ledger empty from", "/api/ledger/exports", `{"to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"ledger bad to", "/api/ledger/exports", `{"from":"2026-11-01","to":"2026/11/30"}`, http.StatusUnprocessableEntity},
		{"report malformed", "/api/reports/renders", `{`, http.StatusBadRequest},
		{"report no id", "/api/reports/renders", `{"from":"2026-11-01","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"report bad from", "/api/reports/renders", `{"report_id":"r","from":"x","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
	}
	stub := &stubExports{id: 1}
	srv := exportServer(t, stub)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, srv.URL+c.path, "carol", c.body)
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
	if stub.calls != 0 {
		t.Fatalf("queue called %d times for refused requests, want 0", stub.calls)
	}
}

// TestExportRoutesQueueFailure: a queue that cannot accept the job is a 500, not a
// pretended 202.
func TestExportRoutesQueueFailure(t *testing.T) {
	stub := &stubExports{err: errors.New("river down")}
	srv := exportServer(t, stub)
	paths := []string{"/api/statistics/exports", "/api/ledger/exports", "/api/reports/renders"}
	bodies := []string{
		`{"state":"bw","from":"2026-11-01","to":"2026-11-30"}`,
		`{"from":"2026-11-01","to":"2026-11-30"}`,
		`{"report_id":"r","from":"2026-11-01","to":"2026-11-30"}`,
	}
	for i, p := range paths {
		resp := doJSON(t, http.MethodPost, srv.URL+p, "carol", bodies[i])
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("%s status = %d, want 500", p, resp.StatusCode)
		}
	}
}

// TestExportRoutesWithoutQueue: with no queue configured the routes say so rather than
// pretending the work was accepted.
func TestExportRoutesWithoutQueue(t *testing.T) {
	srv := exportServer(t, nil)
	for _, p := range []string{"/api/statistics/exports", "/api/ledger/exports", "/api/reports/renders"} {
		resp := doJSON(t, http.MethodPost, srv.URL+p, "carol", `{}`)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want 503", p, resp.StatusCode)
		}
	}
}
