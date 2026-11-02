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

func TestStatisticsExportArgsKind(t *testing.T) {
	if got := (StatisticsExportArgs{}).Kind(); got != KindStatisticsExport {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestStatisticsExportWorkerRuns(t *testing.T) {
	var gotState, gotActor string
	var gotFrom, gotTo time.Time
	w := &StatisticsExportWorker{Export: func(_ context.Context, state string, from, to time.Time, by string) (string, string, error) {
		gotState, gotFrom, gotTo, gotActor = state, from, to, by
		return "bw-20261101-20261130.csv", "Zweck;Einnahmen\nMensa;10.00\n", nil
	}}
	if err := w.Work(context.Background(), &river.Job[StatisticsExportArgs]{
		Args: StatisticsExportArgs{State: " bw ", From: "2026-11-01", To: "2026-11-30", Actor: "alice"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if gotState != "bw" || gotActor != "alice" {
		t.Fatalf("exporter called with state=%q actor=%q", gotState, gotActor)
	}
	if gotFrom.Format(ledgerDate) != "2026-11-01" || gotTo.Format(ledgerDate) != "2026-11-30" {
		t.Fatalf("period = %s..%s", gotFrom.Format(ledgerDate), gotTo.Format(ledgerDate))
	}
}

func TestStatisticsExportWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
		return "stats.csv", "x", nil
	}

	cases := []struct {
		name string
		w    *StatisticsExportWorker
		args StatisticsExportArgs
		want string
	}{
		{"no exporter", &StatisticsExportWorker{}, StatisticsExportArgs{State: "bw"}, "no statistics exporter"},
		{"no state", &StatisticsExportWorker{Export: ok}, StatisticsExportArgs{}, "needs a state"},
		{"bad from", &StatisticsExportWorker{Export: ok}, StatisticsExportArgs{State: "bw", From: "01.11.2026", To: "2026-11-30"}, "period from"},
		{"bad to", &StatisticsExportWorker{Export: ok}, StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "xx"}, "period to"},
		{"exporter fails", &StatisticsExportWorker{Export: func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
			return "", "", boom
		}}, StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "2026-11-30"}, "boom"},
		{"no name", &StatisticsExportWorker{Export: func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
			return " ", "x", nil
		}}, StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "2026-11-30"}, "no file name"},
		{"escaping name", &StatisticsExportWorker{Export: func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
			return "../f.csv", "x", nil
		}}, StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "2026-11-30"}, "not a plain name"},
		{"empty content", &StatisticsExportWorker{Export: func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
			return "stats.csv", "  ", nil
		}}, StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "2026-11-30"}, "no file content"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[StatisticsExportArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestStatisticsExportWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &StatisticsExportWorker{
		Export: func(context.Context, string, time.Time, time.Time, string) (string, string, error) {
			return "stats.csv", "x", nil
		},
		Log: slog.New(slog.NewTextHandler(&buf, nil)),
	}
	args := StatisticsExportArgs{State: "bw", From: "2026-11-01", To: "2026-11-30", Actor: "a"}
	if err := w.Work(context.Background(), &river.Job[StatisticsExportArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "statistics export finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
