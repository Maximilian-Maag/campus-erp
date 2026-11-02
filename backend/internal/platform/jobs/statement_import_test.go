package jobs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/riverqueue/river"
)

func TestStatementImportArgsKind(t *testing.T) {
	if got := (StatementImportArgs{}).Kind(); got != KindStatementImport {
		t.Fatalf("Kind() = %q, want %q", got, KindStatementImport)
	}
}

func TestStatementImportWorkerRuns(t *testing.T) {
	var gotAccount int64
	var gotContent, gotActor string
	w := &StatementImportWorker{Import: func(_ context.Context, accountID int64, content, by string) (int, error) {
		gotAccount, gotContent, gotActor = accountID, content, by
		return 3, nil
	}}
	err := w.Work(context.Background(), &river.Job[StatementImportArgs]{
		Args: StatementImportArgs{AccountID: 7, Content: "<xml/>", Actor: "alice"},
	})
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if gotAccount != 7 || gotContent != "<xml/>" || gotActor != "alice" {
		t.Fatalf("importer called with %d, %q, %q", gotAccount, gotContent, gotActor)
	}
}

func TestStatementImportWorkerErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := func(context.Context, int64, string, string) (int, error) { return 1, nil }

	cases := []struct {
		name string
		w    *StatementImportWorker
		args StatementImportArgs
		want string
	}{
		{"no importer", &StatementImportWorker{}, StatementImportArgs{AccountID: 1, Content: "x"}, "no statement importer"},
		{"no account", &StatementImportWorker{Import: ok}, StatementImportArgs{Content: "x"}, "needs a bank account"},
		{"no content", &StatementImportWorker{Import: ok}, StatementImportArgs{AccountID: 1, Content: "  "}, "needs the file content"},
		{"importer fails", &StatementImportWorker{Import: func(context.Context, int64, string, string) (int, error) { return 0, boom }}, StatementImportArgs{AccountID: 1, Content: "x"}, "boom"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.w.Work(context.Background(), &river.Job[StatementImportArgs]{Args: c.args})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Work error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestStatementImportWorkerUsesProvidedLogger(t *testing.T) {
	var buf strings.Builder
	w := &StatementImportWorker{
		Import: func(context.Context, int64, string, string) (int, error) { return 0, nil },
		Log:    slog.New(slog.NewTextHandler(&buf, nil)),
	}
	if err := w.Work(context.Background(), &river.Job[StatementImportArgs]{Args: StatementImportArgs{AccountID: 1, Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "statement import finished") {
		t.Fatalf("log = %q", buf.String())
	}
}
