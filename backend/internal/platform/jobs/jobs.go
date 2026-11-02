// Package jobs runs asynchronous work on PostgreSQL via River (ADR-0007).
//
// The worker role (cmd/worker) owns the runtime: it applies River's schema,
// registers the job kinds and blocks until asked to stop. The API role reuses the
// same client and args types so it can enqueue work without working it.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// KindETL runs a named ETL dataset (see the registry in internal/platform/etl).
const KindETL = "etl"

// EtlArgs is the payload of a KindETL job.
type EtlArgs struct {
	// Dataset is the registered dataset name.
	Dataset string `json:"dataset"`
	// Actor is the subject the run is attributed to in the audit stream.
	Actor string `json:"actor"`
}

// Kind identifies the job type to River. It implements river.JobArgs.
func (EtlArgs) Kind() string { return KindETL }

// DatasetRunner runs a registered dataset and returns its report. A
// *etl.Registry satisfies it.
type DatasetRunner interface {
	Run(ctx context.Context, name, by string) (etl.Report, error)
}

// EtlWorker works EtlArgs against a dataset registry.
type EtlWorker struct {
	river.WorkerDefaults[EtlArgs]
	Datasets DatasetRunner
	Log      *slog.Logger
}

// Work implements river.Worker. A dataset failure is returned so River retries it.
func (w *EtlWorker) Work(ctx context.Context, job *river.Job[EtlArgs]) error {
	if w.Datasets == nil {
		return errors.New("jobs: no dataset registry configured")
	}
	name := strings.TrimSpace(job.Args.Dataset)
	if name == "" {
		return errors.New("jobs: empty dataset name")
	}
	rep, err := w.Datasets.Run(ctx, name, job.Args.Actor)
	if err != nil {
		return fmt.Errorf("jobs: etl %q: %w", name, err)
	}
	w.log().InfoContext(ctx, "etl job finished",
		"dataset", rep.Dataset, "source", rep.Source, "loaded", rep.Loaded, "drift", rep.Drift)
	return nil
}

func (w *EtlWorker) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// Migrate applies River's schema to the pool. It is idempotent and safe to run on
// every start.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("jobs: migrator: %w", err)
	}
	if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("jobs: migrate: %w", err)
	}
	return nil
}

// StandardWorkers is the set of job kinds the worker role runs.
func StandardWorkers(datasets DatasetRunner, importStatement StatementImportFunc,
	exportLedger LedgerExportFunc, exportSepa SepaExportFunc, renderReport ReportRenderFunc) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &EtlWorker{Datasets: datasets})
	river.AddWorker(workers, &StatementImportWorker{Import: importStatement})
	river.AddWorker(workers, &LedgerExportWorker{Export: exportLedger})
	river.AddWorker(workers, &SepaExportWorker{Export: exportSepa})
	river.AddWorker(workers, &ReportRenderWorker{Render: renderReport})
	return workers
}

// New builds a River client that works the standard job kinds over pool. The
// caller owns the pool and must call Migrate before Start.
func New(pool *pgxpool.Pool, datasets DatasetRunner, importStatement StatementImportFunc,
	exportLedger LedgerExportFunc, exportSepa SepaExportFunc, renderReport ReportRenderFunc) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: StandardWorkers(datasets, importStatement, exportLedger, exportSepa, renderReport),
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: client: %w", err)
	}
	return client, nil
}

// EnqueueEtl inserts a KindETL job and returns its id. The API role uses it to hand
// an ETL run to the worker.
func EnqueueEtl(ctx context.Context, client *river.Client[pgx.Tx], dataset, actor string) (int64, error) {
	res, err := client.Insert(ctx, EtlArgs{Dataset: dataset, Actor: actor}, nil)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue etl: %w", err)
	}
	return res.Job.ID, nil
}
