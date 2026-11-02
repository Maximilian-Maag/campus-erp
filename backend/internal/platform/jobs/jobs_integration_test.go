package jobs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/rivertype"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

func jobsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return pool
}

// TestWorkerRunsEtlJob exercises the whole runtime: River's schema is applied, a
// client works the etl kind, a job is enqueued and completes, and the worker
// receives the args it was queued with.
func TestWorkerRunsEtlJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := jobsTestPool(t)
	if err := jobs.Migrate(ctx, pool); err != nil {
		t.Fatalf("jobs.Migrate: %v", err)
	}

	stub := &stubDatasets{}
	client, err := jobs.New(pool, stub, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	id, err := jobs.EnqueueEtl(ctx, client, "bank-directory", "alice")
	if err != nil {
		t.Fatalf("EnqueueEtl: %v", err)
	}
	if id == 0 {
		t.Fatal("EnqueueEtl returned job id 0")
	}

	// Poll for completion: the job must reach the completed state and the worker
	// must have received the queued args.
	deadline := time.Now().Add(20 * time.Second)
	for {
		job, err := client.JobGet(ctx, id)
		if err != nil {
			t.Fatalf("JobGet: %v", err)
		}
		if job.State == rivertype.JobStateCompleted {
			break
		}
		if job.State == rivertype.JobStateDiscarded || job.State == rivertype.JobStateCancelled {
			t.Fatalf("job %d ended in state %q", id, job.State)
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d stuck in state %q", id, job.State)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if stub.calls != 1 || stub.name != "bank-directory" || stub.actor != "alice" {
		t.Fatalf("worker got (%d, %q, %q), want (1, bank-directory, alice)", stub.calls, stub.name, stub.actor)
	}
}

// TestWorkerRunsStatisticsExportJob exercises the statistics-export kind through the
// real runtime: the kind is registered on the client, the job is enqueued, the worker
// works it, and the render seam receives the queued period and state.
func TestWorkerRunsStatisticsExportJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := jobsTestPool(t)
	if err := jobs.Migrate(ctx, pool); err != nil {
		t.Fatalf("jobs.Migrate: %v", err)
	}

	var calls int
	var gotState, gotActor string
	var gotFrom, gotTo time.Time
	export := func(_ context.Context, state string, from, to time.Time, by string) (string, string, error) {
		calls++
		gotState, gotFrom, gotTo, gotActor = state, from, to, by
		return "bw-20261101-20261130.csv", "Zweck;Einnahmen\nMensa;10.00\n", nil
	}
	client, err := jobs.New(pool, nil, nil, nil, nil, nil, export)
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	id, err := jobs.EnqueueStatisticsExport(ctx, client, "bw", "2026-11-01", "2026-11-30", "alice")
	if err != nil {
		t.Fatalf("EnqueueStatisticsExport: %v", err)
	}
	if id == 0 {
		t.Fatal("EnqueueStatisticsExport returned job id 0")
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		job, err := client.JobGet(ctx, id)
		if err != nil {
			t.Fatalf("JobGet: %v", err)
		}
		if job.State == rivertype.JobStateCompleted {
			break
		}
		if job.State == rivertype.JobStateDiscarded || job.State == rivertype.JobStateCancelled {
			t.Fatalf("job %d ended in state %q", id, job.State)
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d stuck in state %q", id, job.State)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if calls != 1 || gotState != "bw" || gotActor != "alice" {
		t.Fatalf("exporter got (%d, %q, %q), want (1, bw, alice)", calls, gotState, gotActor)
	}
	if gotFrom.Format("2006-01-02") != "2026-11-01" || gotTo.Format("2006-01-02") != "2026-11-30" {
		t.Fatalf("period = %s..%s", gotFrom.Format("2006-01-02"), gotTo.Format("2006-01-02"))
	}
}
