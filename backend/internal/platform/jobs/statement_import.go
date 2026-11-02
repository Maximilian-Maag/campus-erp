package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// KindStatementImport imports one bank-statement file into the cash book.
const KindStatementImport = "statement-import"

// StatementImportArgs is the payload of a KindStatementImport job. The file content
// travels with the job: statement files are small, and keeping the payload self
// contained means a retry re-imports exactly the same bytes.
type StatementImportArgs struct {
	AccountID int64  `json:"account_id"`
	Content   string `json:"content"`
	Actor     string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (StatementImportArgs) Kind() string { return KindStatementImport }

// StatementImportFunc imports one statement file and reports how many statement lines
// it produced. It keeps this package free of a dependency on the module that owns the
// import, like DatasetRunner does for ETL.
type StatementImportFunc func(ctx context.Context, accountID int64, content, by string) (int, error)

// StatementImportWorker works StatementImportArgs.
type StatementImportWorker struct {
	river.WorkerDefaults[StatementImportArgs]
	Import StatementImportFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it; the import
// itself records its statements on the audit stream.
func (w *StatementImportWorker) Work(ctx context.Context, job *river.Job[StatementImportArgs]) error {
	if w.Import == nil {
		return errors.New("jobs: no statement importer configured")
	}
	if job.Args.AccountID <= 0 {
		return errors.New("jobs: statement import needs a bank account")
	}
	if strings.TrimSpace(job.Args.Content) == "" {
		return errors.New("jobs: statement import needs the file content")
	}
	n, err := w.Import(ctx, job.Args.AccountID, job.Args.Content, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: statement import: %w", err)
	}
	w.log().InfoContext(ctx, "statement import finished", "account", job.Args.AccountID, "statements", n)
	return nil
}

func (w *StatementImportWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// EnqueueStatementImport inserts a KindStatementImport job and returns its id. The API
// role uses it to hand an uploaded statement file to the worker.
func EnqueueStatementImport(ctx context.Context, client *river.Client[pgx.Tx], accountID int64, content, actor string) (int64, error) {
	res, err := client.Insert(ctx, StatementImportArgs{AccountID: accountID, Content: content, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue statement import: %w", err)
	}
	return res.Job.ID, nil
}
