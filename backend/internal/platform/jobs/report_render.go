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

// KindReportRender renders a report and stores the artefact.
const KindReportRender = "report-render"

// ReportRenderArgs is the payload of a KindReportRender job: which report, over which
// period (inclusive, YYYY-MM-DD), for whom.
type ReportRenderArgs struct {
	ReportID string `json:"report_id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Actor    string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (ReportRenderArgs) Kind() string { return KindReportRender }

// ReportRenderFunc builds a report by id and stores its artefact, returning the key it
// was stored under. It keeps this package free of a dependency on the domains that own
// the reports.
type ReportRenderFunc func(ctx context.Context, reportID string, from, to time.Time, by string) (string, error)

// ReportRenderWorker works ReportRenderArgs.
type ReportRenderWorker struct {
	river.WorkerDefaults[ReportRenderArgs]
	Render ReportRenderFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it.
func (w *ReportRenderWorker) Work(ctx context.Context, job *river.Job[ReportRenderArgs]) error {
	if w.Render == nil {
		return errors.New("jobs: no report renderer configured")
	}
	id := strings.TrimSpace(job.Args.ReportID)
	if id == "" {
		return errors.New("jobs: report render needs a report id")
	}
	from, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.From))
	if err != nil {
		return fmt.Errorf("jobs: report render period from %q: %w", job.Args.From, err)
	}
	to, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.To))
	if err != nil {
		return fmt.Errorf("jobs: report render period to %q: %w", job.Args.To, err)
	}
	key, err := w.Render(ctx, id, from, to, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: report render %q: %w", id, err)
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("jobs: report render stored nothing")
	}
	w.log().InfoContext(ctx, "report render finished", "report", id, "key", key)
	return nil
}

func (w *ReportRenderWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// EnqueueReportRender inserts a KindReportRender job and returns its id.
func EnqueueReportRender(ctx context.Context, client *river.Client[pgx.Tx], reportID, from, to, actor string) (int64, error) {
	res, err := client.Insert(ctx, ReportRenderArgs{ReportID: reportID, From: from, To: to, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue report render: %w", err)
	}
	return res.Job.ID, nil
}
