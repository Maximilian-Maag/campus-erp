package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/riverqueue/river"
)

// KindSepaExport hands a payment run's SEPA file to the bank channel.
const KindSepaExport = "sepa-export"

// SepaExportArgs is the payload of a KindSepaExport job. The run already holds its
// rendered pain.001 file; the job delivers it.
type SepaExportArgs struct {
	RunID int64  `json:"run_id"`
	Actor string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (SepaExportArgs) Kind() string { return KindSepaExport }

// SepaExportFunc returns the file name and the bytes of a payment run's SEPA file. It
// keeps this package free of a dependency on the cash-book module.
type SepaExportFunc func(ctx context.Context, runID int64, by string) (name, content string, err error)

// SepaExportWorker works SepaExportArgs.
type SepaExportWorker struct {
	river.WorkerDefaults[SepaExportArgs]
	Export SepaExportFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it.
func (w *SepaExportWorker) Work(ctx context.Context, job *river.Job[SepaExportArgs]) error {
	if w.Export == nil {
		return errors.New("jobs: no SEPA exporter configured")
	}
	if job.Args.RunID <= 0 {
		return errors.New("jobs: SEPA export needs a payment run")
	}
	name, content, err := w.Export(ctx, job.Args.RunID, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: SEPA export: %w", err)
	}
	if err := plainFileName(name); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("jobs: SEPA export produced no file content")
	}
	w.log().InfoContext(ctx, "SEPA export finished", "run", job.Args.RunID, "file", name)
	return nil
}

// plainFileName refuses a file name that would escape the outbox. Both export kinds
// use it, so the rule and its wording are in one place.
func plainFileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("jobs: export produced no file name")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `\/`) {
		return fmt.Errorf("jobs: export file name %q is not a plain name", name)
	}
	return nil
}

func (w *SepaExportWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}
