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

// KindReturnPostings renders a period's return postings and hands them to the ledger
// side (docs/30-return-postings.md).
const KindReturnPostings = "return-postings"

// ReturnPostingsArgs is the payload of a KindReturnPostings job.
type ReturnPostingsArgs struct {
	// From and To are the period, inclusive, as YYYY-MM-DD.
	From  string `json:"from"`
	To    string `json:"to"`
	Actor string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (ReturnPostingsArgs) Kind() string { return KindReturnPostings }

// ReturnPostingsFunc renders a period's return postings and hands them over, reporting
// how many postings it carried. Keeping it a function means this package neither imports
// the cash-book module nor knows where the file goes.
type ReturnPostingsFunc func(ctx context.Context, from, to time.Time, by string) (int, error)

// ReturnPostingsWorker works ReturnPostingsArgs.
type ReturnPostingsWorker struct {
	river.WorkerDefaults[ReturnPostingsArgs]
	Export ReturnPostingsFunc
	Log    *slog.Logger
}

// Work implements river.Worker. A failure is returned so River retries it.
func (w *ReturnPostingsWorker) Work(ctx context.Context, job *river.Job[ReturnPostingsArgs]) error {
	if w.Export == nil {
		return errors.New("jobs: no return-postings exporter configured")
	}
	if strings.TrimSpace(job.Args.Actor) == "" {
		return errors.New("jobs: return postings need an actor")
	}
	from, err := time.Parse(PeriodLayout, strings.TrimSpace(job.Args.From))
	if err != nil {
		return fmt.Errorf("jobs: return-postings period from %q: %w", job.Args.From, err)
	}
	to, err := time.Parse(PeriodLayout, strings.TrimSpace(job.Args.To))
	if err != nil {
		return fmt.Errorf("jobs: return-postings period to %q: %w", job.Args.To, err)
	}
	n, err := w.Export(ctx, from, to, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: return postings: %w", err)
	}
	w.log().InfoContext(ctx, "return postings finished",
		"from", from.Format(PeriodLayout), "to", to.Format(PeriodLayout), "postings", n)
	return nil
}

func (w *ReturnPostingsWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// EnqueueReturnPostings inserts a KindReturnPostings job and returns its id.
func EnqueueReturnPostings(ctx context.Context, client *river.Client[pgx.Tx], from, to, actor string) (int64, error) {
	res, err := client.Insert(ctx, ReturnPostingsArgs{From: from, To: to, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue return postings: %w", err)
	}
	return res.Job.ID, nil
}
