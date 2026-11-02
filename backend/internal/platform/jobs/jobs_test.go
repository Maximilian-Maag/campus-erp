package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

var errBoom = errors.New("boom")

type stubDatasets struct {
	name   string
	actor  string
	calls  int
	report etl.Report
	err    error
}

func (s *stubDatasets) Run(_ context.Context, name, by string) (etl.Report, error) {
	s.calls++
	s.name, s.actor = name, by
	if s.err != nil {
		return etl.Report{}, s.err
	}
	return s.report, nil
}

func etlJob(dataset, actor string) *river.Job[jobs.EtlArgs] {
	return &river.Job[jobs.EtlArgs]{Args: jobs.EtlArgs{Dataset: dataset, Actor: actor}}
}

func TestEtlArgsKind(t *testing.T) {
	// The kind string is persisted in the queue, so pin the wire value: renaming it
	// would orphan every queued job.
	if jobs.KindETL != "etl" {
		t.Fatalf("KindETL = %q, want \"etl\"", jobs.KindETL)
	}
	if got := (jobs.EtlArgs{}).Kind(); got != jobs.KindETL {
		t.Fatalf("EtlArgs.Kind() = %q, want %q", got, jobs.KindETL)
	}
}

func TestEtlWorkerWork(t *testing.T) {
	stub := &stubDatasets{report: etl.Report{Dataset: "bank-directory", Source: 3, Loaded: 3}}
	w := &jobs.EtlWorker{Datasets: stub}

	if err := w.Work(context.Background(), etlJob("bank-directory", "alice")); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if stub.calls != 1 || stub.name != "bank-directory" || stub.actor != "alice" {
		t.Fatalf("runner got (%d, %q, %q)", stub.calls, stub.name, stub.actor)
	}
}

func TestEtlWorkerWorkTrimsName(t *testing.T) {
	stub := &stubDatasets{}
	w := &jobs.EtlWorker{Datasets: stub}

	if err := w.Work(context.Background(), etlJob("  bank-directory  ", "bob")); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if stub.name != "bank-directory" {
		t.Fatalf("name = %q, want trimmed", stub.name)
	}
}

func TestEtlWorkerWorkErrors(t *testing.T) {
	if err := (&jobs.EtlWorker{}).Work(context.Background(), etlJob("x", "a")); err == nil {
		t.Fatal("nil registry must error")
	}
	w := &jobs.EtlWorker{Datasets: &stubDatasets{}}
	if err := w.Work(context.Background(), etlJob("   ", "a")); err == nil {
		t.Fatal("blank dataset must error")
	}
	failing := &jobs.EtlWorker{Datasets: &stubDatasets{err: errBoom}}
	if err := failing.Work(context.Background(), etlJob("x", "a")); !errors.Is(err, errBoom) {
		t.Fatalf("runner error must be wrapped: %v", err)
	}
}
