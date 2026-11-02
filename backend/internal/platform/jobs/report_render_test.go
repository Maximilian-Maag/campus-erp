package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

func TestReportRenderArgsKind(t *testing.T) {
	if got := (ReportRenderArgs{}).Kind(); got != KindReportRender {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestReportRenderWorkerRuns(t *testing.T) {
	var gotID, gotActor string
	var gotFrom, gotTo time.Time
	w := &ReportRenderWorker{Render: func(_ context.Context, id string, from, to time.Time, by string) (string, error) {
		gotID, gotFrom, gotTo, gotActor = id, from, to, by
		return "reports/kasse-daily-close.csv", nil
	}}
	if err := w.Work(context.Background(), &river.Job[ReportRenderArgs]{
		Args: ReportRenderArgs{ReportID: " kasse-daily-close ", From: "2026-11-01", To: "2026-11-30", Actor: "alice"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if gotID != "kasse-daily-close" || !gotFrom.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) ||
		!gotTo.Equal(time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)) || gotActor != "alice" {
		t.Fatalf("renderer called with %q, %v, %v, %q", gotID, gotFrom, gotTo, gotActor)
	}
}

func TestReportRenderWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, string, time.Time, time.Time, string) (string, error) { return "k", nil }

	cases := []struct {
		name string
		w    *ReportRenderWorker
		args ReportRenderArgs
		want string
	}{
		{"no renderer", &ReportRenderWorker{}, ReportRenderArgs{ReportID: "r", From: "2026-11-01", To: "2026-11-02"}, "no report renderer"},
		{"no report id", &ReportRenderWorker{Render: ok}, ReportRenderArgs{From: "2026-11-01", To: "2026-11-02"}, "needs a report id"},
		{"bad from", &ReportRenderWorker{Render: ok}, ReportRenderArgs{ReportID: "r", From: "01.11.2026", To: "2026-11-02"}, "period from"},
		{"bad to", &ReportRenderWorker{Render: ok}, ReportRenderArgs{ReportID: "r", From: "2026-11-01"}, "period to"},
		{"renderer fails", &ReportRenderWorker{Render: func(context.Context, string, time.Time, time.Time, string) (string, error) { return "", boom }}, ReportRenderArgs{ReportID: "r", From: "2026-11-01", To: "2026-11-02"}, "boom"},
		{"nothing stored", &ReportRenderWorker{Render: func(context.Context, string, time.Time, time.Time, string) (string, error) { return "  ", nil }}, ReportRenderArgs{ReportID: "r", From: "2026-11-01", To: "2026-11-02"}, "stored nothing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[ReportRenderArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestReportRenderWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &ReportRenderWorker{
		Render: func(context.Context, string, time.Time, time.Time, string) (string, error) { return "k", nil },
		Log:    slog.New(slog.NewTextHandler(&buf, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[ReportRenderArgs]{
		Args: ReportRenderArgs{ReportID: "r", From: "2026-11-01", To: "2026-11-02", Actor: "a"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "report render finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
