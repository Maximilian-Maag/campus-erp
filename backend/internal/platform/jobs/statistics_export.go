package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// KindStatisticsExport renders one state's statistics file for a period (docs/27).
const KindStatisticsExport = "statistics-export"

// StatisticsExportArgs is the payload of a KindStatisticsExport job: which state's
// layout, over which period (inclusive, YYYY-MM-DD), for whom.
type StatisticsExportArgs struct {
	State string `json:"state"`
	From  string `json:"from"`
	To    string `json:"to"`
	Actor string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (StatisticsExportArgs) Kind() string { return KindStatisticsExport }

// StatisticsExportFunc renders one state's statistics file and stores it, returning the
// file name and its bytes. It keeps this package free of a dependency on the domains
// that own the figures.
type StatisticsExportFunc func(ctx context.Context, state string, from, to time.Time, by string) (name, content string, err error)

// StatisticsExportWorker works StatisticsExportArgs.
type StatisticsExportWorker struct {
	river.WorkerDefaults[StatisticsExportArgs]
	Export StatisticsExportFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it.
func (w *StatisticsExportWorker) Work(ctx context.Context, job *river.Job[StatisticsExportArgs]) error {
	if w.Export == nil {
		return errors.New("jobs: no statistics exporter configured")
	}
	state := strings.TrimSpace(job.Args.State)
	if state == "" {
		return errors.New("jobs: statistics export needs a state")
	}
	from, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.From))
	if err != nil {
		return fmt.Errorf("jobs: statistics export period from %q: %w", job.Args.From, err)
	}
	to, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.To))
	if err != nil {
		return fmt.Errorf("jobs: statistics export period to %q: %w", job.Args.To, err)
	}
	name, content, err := w.Export(ctx, state, from, to, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: statistics export %q: %w", state, err)
	}
	if err := plainFileName(name); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return errors.New("jobs: statistics export produced no file content")
	}
	w.log().InfoContext(ctx, "statistics export finished",
		"state", state, "from", from.Format(ledgerDate), "to", to.Format(ledgerDate), "file", name)
	return nil
}

func (w *StatisticsExportWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// EnqueueStatisticsExport inserts a KindStatisticsExport job and returns its id.
func EnqueueStatisticsExport(ctx context.Context, client *river.Client[pgx.Tx], state, from, to, actor string) (int64, error) {
	res, err := client.Insert(ctx, StatisticsExportArgs{State: state, From: from, To: to, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue statistics export: %w", err)
	}
	return res.Job.ID, nil
}
