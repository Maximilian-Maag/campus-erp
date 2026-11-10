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

func TestReturnPostingsArgsKind(t *testing.T) {
	if got := (ReturnPostingsArgs{}).Kind(); got != KindReturnPostings {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestReturnPostingsWorkerRuns(t *testing.T) {
	var gotFrom, gotTo time.Time
	var gotActor string
	w := &ReturnPostingsWorker{Export: func(_ context.Context, from, to time.Time, by string) (int, error) {
		gotFrom, gotTo, gotActor = from, to, by
		return 2, nil
	}}
	if err := w.Work(context.Background(), &river.Job[ReturnPostingsArgs]{
		Args: ReturnPostingsArgs{From: "2026-10-01", To: "2026-10-31", Actor: "alice"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if !gotFrom.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		!gotTo.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) || gotActor != "alice" {
		t.Fatalf("exporter called with %v, %v, %q", gotFrom, gotTo, gotActor)
	}
}

func TestReturnPostingsWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, time.Time, time.Time, string) (int, error) { return 1, nil }

	cases := []struct {
		name string
		w    *ReturnPostingsWorker
		args ReturnPostingsArgs
		want string
	}{
		{"no exporter", &ReturnPostingsWorker{}, ReturnPostingsArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"}, "no return-postings exporter"},
		{"no actor", &ReturnPostingsWorker{Export: ok}, ReturnPostingsArgs{From: "2026-10-01", To: "2026-10-02"}, "need an actor"},
		{"bad from", &ReturnPostingsWorker{Export: ok}, ReturnPostingsArgs{From: "01.10.2026", To: "2026-10-02", Actor: "a"}, "period from"},
		{"bad to", &ReturnPostingsWorker{Export: ok}, ReturnPostingsArgs{From: "2026-10-01", To: "", Actor: "a"}, "period to"},
		{"exporter fails", &ReturnPostingsWorker{Export: func(context.Context, time.Time, time.Time, string) (int, error) { return 0, boom }}, ReturnPostingsArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"}, "boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[ReturnPostingsArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestReturnPostingsWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &ReturnPostingsWorker{
		Export: func(context.Context, time.Time, time.Time, string) (int, error) { return 0, nil },
		Log:    slog.New(slog.NewTextHandler(&buf, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[ReturnPostingsArgs]{
		Args: ReturnPostingsArgs{From: "2026-10-01", To: "2026-10-02", Actor: "a"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "return postings finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
