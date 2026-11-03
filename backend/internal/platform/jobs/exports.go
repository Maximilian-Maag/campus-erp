package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// NewEnqueuer builds an insert-only River client: a role that hands work to the worker
// without working any job itself. A client with no queues and no workers is insert-only
// by construction (River refuses a client whose queues are set without workers), so the
// API role can enqueue without ever racing the worker for work.
func NewEnqueuer(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("jobs: enqueuer: %w", err)
	}
	return client, nil
}

// Exports is the enqueue seam the HTTP layer uses. It exposes the enqueue operations with
// plain arguments, so the transport does not depend on River and can be tested with a
// stub — the same reason the workers take func seams rather than domain types.
type Exports struct{ client *river.Client[pgx.Tx] }

// NewExports wraps an insert-only client built by NewEnqueuer.
func NewExports(client *river.Client[pgx.Tx]) Exports { return Exports{client: client} }

// EnqueueStatisticsExport queues a per-state statistics export for the period [from, to]
// (both YYYY-MM-DD) and returns the job id.
func (e Exports) EnqueueStatisticsExport(ctx context.Context, state, from, to, actor string) (int64, error) {
	return EnqueueStatisticsExport(ctx, e.client, state, from, to, actor)
}

// EnqueueLedgerExport queues a ledger posting export for the period [from, to] and returns
// the job id.
func (e Exports) EnqueueLedgerExport(ctx context.Context, from, to, actor string) (int64, error) {
	return EnqueueLedgerExport(ctx, e.client, from, to, actor)
}

// EnqueueReportRender queues a report render by id over [from, to] and returns the job id.
func (e Exports) EnqueueReportRender(ctx context.Context, reportID, from, to, actor string) (int64, error) {
	return EnqueueReportRender(ctx, e.client, reportID, from, to, actor)
}

// EnqueueStatementImport queues a bank-statement import of content for an account and
// returns the job id. The bytes travel with the job, so a retry re-imports the same file.
func (e Exports) EnqueueStatementImport(ctx context.Context, accountID int64, content, actor string) (int64, error) {
	return EnqueueStatementImport(ctx, e.client, accountID, content, actor)
}

// EnqueueSepaExport queues the delivery of a payment run's SEPA file and returns the job id.
func (e Exports) EnqueueSepaExport(ctx context.Context, runID int64, actor string) (int64, error) {
	return EnqueueSepaExport(ctx, e.client, runID, actor)
}
