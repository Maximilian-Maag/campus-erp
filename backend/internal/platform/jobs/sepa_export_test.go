package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/riverqueue/river"
)

func TestSepaExportArgsKind(t *testing.T) {
	if got := (SepaExportArgs{}).Kind(); got != KindSepaExport {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestSepaExportWorkerRuns(t *testing.T) {
	var gotRun int64
	var gotActor string
	w := &SepaExportWorker{Export: func(_ context.Context, runID int64, by string) (string, string, error) {
		gotRun, gotActor = runID, by
		return "pain001.xml", "<Document/>", nil
	}}
	if err := w.Work(context.Background(), &river.Job[SepaExportArgs]{
		Args: SepaExportArgs{RunID: 7, Actor: "alice"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if gotRun != 7 || gotActor != "alice" {
		t.Fatalf("exporter called with %d, %q", gotRun, gotActor)
	}
}

func TestSepaExportWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, int64, string) (string, string, error) { return "f.xml", "<x/>", nil }

	cases := []struct {
		name string
		w    *SepaExportWorker
		args SepaExportArgs
		want string
	}{
		{"no exporter", &SepaExportWorker{}, SepaExportArgs{RunID: 1}, "no SEPA exporter"},
		{"no run", &SepaExportWorker{Export: ok}, SepaExportArgs{}, "needs a payment run"},
		{"exporter fails", &SepaExportWorker{Export: func(context.Context, int64, string) (string, string, error) { return "", "", boom }}, SepaExportArgs{RunID: 1}, "boom"},
		{"no name", &SepaExportWorker{Export: func(context.Context, int64, string) (string, string, error) { return " ", "<x/>", nil }}, SepaExportArgs{RunID: 1}, "no file name"},
		{"empty content", &SepaExportWorker{Export: func(context.Context, int64, string) (string, string, error) { return "f.xml", "  ", nil }}, SepaExportArgs{RunID: 1}, "no file content"},
		{"escaping name", &SepaExportWorker{Export: func(context.Context, int64, string) (string, string, error) { return "../f.xml", "<x/>", nil }}, SepaExportArgs{RunID: 1}, "not a plain name"},
		{"nested name", &SepaExportWorker{Export: func(context.Context, int64, string) (string, string, error) { return "a/f.xml", "<x/>", nil }}, SepaExportArgs{RunID: 1}, "not a plain name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[SepaExportArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestSepaExportWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &SepaExportWorker{
		Export: func(context.Context, int64, string) (string, string, error) { return "f.xml", "<x/>", nil },
		Log:    slog.New(slog.NewTextHandler(&buf, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[SepaExportArgs]{Args: SepaExportArgs{RunID: 1, Actor: "a"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "SEPA export finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
