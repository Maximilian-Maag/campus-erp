package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
)

// stubExports records what a queued statistics export was asked for.
type stubExports struct {
	calls                              int
	gotState, gotFrom, gotTo, gotActor string
	id                                 int64
	err                                error
}

func (s *stubExports) EnqueueStatisticsExport(_ context.Context, state, from, to, actor string) (int64, error) {
	s.calls++
	s.gotState, s.gotFrom, s.gotTo, s.gotActor = state, from, to, actor
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

func TestStatisticsExportRouteQueues(t *testing.T) {
	stub := &stubExports{id: 42}
	srv := exportServer(t, stub)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/statistics/exports", "carol",
		`{"state":"bw","from":"2026-11-01","to":"2026-11-30"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var got struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.JobID != 42 {
		t.Fatalf("job_id = %d, want 42", got.JobID)
	}
	if stub.calls != 1 || stub.gotState != "bw" || stub.gotFrom != "2026-11-01" ||
		stub.gotTo != "2026-11-30" || stub.gotActor != "carol" {
		t.Fatalf("queue got (%d, %q, %q, %q, %q), want (1, bw, 2026-11-01, 2026-11-30, carol)",
			stub.calls, stub.gotState, stub.gotFrom, stub.gotTo, stub.gotActor)
	}
}

func TestStatisticsExportRouteRefusals(t *testing.T) {
	stub := &stubExports{id: 1}
	srv := exportServer(t, stub)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"malformed json", `{`, http.StatusBadRequest},
		{"no state", `{"from":"2026-11-01","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"blank state", `{"state":"  ","from":"2026-11-01","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"bad from", `{"state":"bw","from":"01.11.2026","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"empty from", `{"state":"bw","to":"2026-11-30"}`, http.StatusUnprocessableEntity},
		{"bad to", `{"state":"bw","from":"2026-11-01","to":"nope"}`, http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := doJSON(t, http.MethodPost, srv.URL+"/api/statistics/exports", "carol", c.body)
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
	if stub.calls != 0 {
		t.Fatalf("queue called %d times for refused requests, want 0", stub.calls)
	}
}

func TestStatisticsExportRouteQueueFailure(t *testing.T) {
	stub := &stubExports{err: errors.New("river down")}
	srv := exportServer(t, stub)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/statistics/exports", "carol",
		`{"state":"bw","from":"2026-11-01","to":"2026-11-30"}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

// TestStatisticsExportRouteWithoutQueue: with no queue configured the route is honest
// about it instead of pretending the export was queued.
func TestStatisticsExportRouteWithoutQueue(t *testing.T) {
	srv := exportServer(t, nil)
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/statistics/exports", "carol",
		`{"state":"bw","from":"2026-11-01","to":"2026-11-30"}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("expected a problem body")
	}
}
