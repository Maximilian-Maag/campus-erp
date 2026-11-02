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

// KindLedgerExport renders a period's posting export and hands it to the ledger side.
const KindLedgerExport = "ledger-export"

// ledgerDate is the layout the job carries its period in.
const ledgerDate = "2006-01-02"

// LedgerExportArgs is the payload of a KindLedgerExport job.
type LedgerExportArgs struct {
	// From and To are the period, inclusive, as YYYY-MM-DD.
	From  string `json:"from"`
	To    string `json:"to"`
	Actor string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (LedgerExportArgs) Kind() string { return KindLedgerExport }

// LedgerExportFunc renders a period's export and hands it over, reporting how many
// postings it carried. Keeping it a function means this package neither imports the
// cash-book module nor knows where the file goes.
type LedgerExportFunc func(ctx context.Context, from, to time.Time, by string) (int, error)

// LedgerExportWorker works LedgerExportArgs.
type LedgerExportWorker struct {
	river.WorkerDefaults[LedgerExportArgs]
	Export LedgerExportFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it.
func (w *LedgerExportWorker) Work(ctx context.Context, job *river.Job[LedgerExportArgs]) error {
	if w.Export == nil {
		return errors.New("jobs: no ledger exporter configured")
	}
	if strings.TrimSpace(job.Args.Actor) == "" {
		return errors.New("jobs: ledger export needs an actor")
	}
	from, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.From))
	if err != nil {
		return fmt.Errorf("jobs: ledger export period from %q: %w", job.Args.From, err)
	}
	to, err := time.Parse(ledgerDate, strings.TrimSpace(job.Args.To))
	if err != nil {
		return fmt.Errorf("jobs: ledger export period to %q: %w", job.Args.To, err)
	}
	n, err := w.Export(ctx, from, to, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: ledger export: %w", err)
	}
	w.log().InfoContext(ctx, "ledger export finished",
		"from", from.Format(ledgerDate), "to", to.Format(ledgerDate), "postings", n)
	return nil
}

func (w *LedgerExportWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// EnqueueLedgerExport inserts a KindLedgerExport job and returns its id.
func EnqueueLedgerExport(ctx context.Context, client *river.Client[pgx.Tx], from, to, actor string) (int64, error) {
	res, err := client.Insert(ctx, LedgerExportArgs{From: from, To: to, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue ledger export: %w", err)
	}
	return res.Job.ID, nil
}
