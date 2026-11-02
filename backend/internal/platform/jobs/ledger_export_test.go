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

func TestLedgerExportArgsKind(t *testing.T) {
	if got := (LedgerExportArgs{}).Kind(); got != KindLedgerExport {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestLedgerExportWorkerRuns(t *testing.T) {
	var gotFrom, gotTo time.Time
	var gotActor string
	w := &LedgerExportWorker{Export: func(_ context.Context, from, to time.Time, by string) (int, error) {
		gotFrom, gotTo, gotActor = from, to, by
		return 4, nil
	}}
	if err := w.Work(context.Background(), &river.Job[LedgerExportArgs]{
		Args: LedgerExportArgs{From: "2026-10-01", To: "2026-10-31", Actor: "alice"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if !gotFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		!gotTo.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) || gotActor != "alice" {
		t.Fatalf("exporter called with %v, %v, %q", gotFrom, gotTo, gotActor)
	}
}

func TestLedgerExportWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, time.Time, time.Time, string) (int, error) { return 1, nil }

	cases := []struct {
		name string
		w    *LedgerExportWorker
		args LedgerExportArgs
		want string
	}{
		{"no exporter", &LedgerExportWorker{}, LedgerExportArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"}, "no ledger exporter"},
		{"no actor", &LedgerExportWorker{Export: ok}, LedgerExportArgs{From: "2026-10-01", To: "2026-10-02"}, "needs an actor"},
		{"bad from", &LedgerExportWorker{Export: ok}, LedgerExportArgs{From: "01.10.2026", To: "2026-10-02", Actor: "a"}, "period from"},
		{"bad to", &LedgerExportWorker{Export: ok}, LedgerExportArgs{From: "2026-10-01", To: "", Actor: "a"}, "period to"},
		{"exporter fails", &LedgerExportWorker{Export: func(context.Context, time.Time, time.Time, string) (int, error) { return 0, boom }}, LedgerExportArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"}, "boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[LedgerExportArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestLedgerExportWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &LedgerExportWorker{
		Export: func(context.Context, time.Time, time.Time, string) (int, error) { return 0, nil },
		Log:    slog.New(slog.NewTextHandler(&buf, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[LedgerExportArgs]{
		Args: LedgerExportArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ledger export finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
