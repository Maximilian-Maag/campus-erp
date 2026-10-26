package lookup

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

type stubRepo struct {
	saved []Value
	err   error
}

func (r *stubRepo) Upsert(_ context.Context, v Value) (Value, error) {
	if r.err != nil {
		return Value{}, r.err
	}
	r.saved = append(r.saved, v)
	return v, nil
}

func (r *stubRepo) Get(context.Context, string, string) (Value, error) { return Value{}, ErrNotFound }
func (r *stubRepo) List(context.Context, string) ([]Value, error)      { return nil, nil }

type stubSink struct{ err error }

func (s stubSink) Append(_ context.Context, e audit.Event) (audit.Event, error) { return e, s.err }

func TestUpsertReturnsSavedValue(t *testing.T) {
	repo := &stubRepo{}
	svc := NewValueService(repo, stubSink{})
	got, err := svc.Upsert(context.Background(), Value{Namespace: "ns", Code: "A", ShortText: "alpha", Active: true}, "alice")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Namespace != "ns" || got.Code != "A" || got.ShortText != "alpha" {
		t.Fatalf("returned value = %+v", got)
	}
}

func TestUpsertErrors(t *testing.T) {
	svc := NewValueService(&stubRepo{}, stubSink{err: errors.New("audit down")})
	if _, err := svc.Upsert(context.Background(), Value{Namespace: "ns", Code: "A", ShortText: "x"}, "alice"); err == nil {
		t.Fatal("expected the audit error to propagate")
	}
	svc2 := NewValueService(&stubRepo{err: errors.New("db down")}, stubSink{})
	if _, err := svc2.Upsert(context.Background(), Value{Namespace: "ns", Code: "A", ShortText: "x"}, "alice"); err == nil {
		t.Fatal("expected the repo error to propagate")
	}
}

func TestUpsertManyReturnsSavedValues(t *testing.T) {
	svc := NewValueService(&stubRepo{}, stubSink{})
	got, err := svc.UpsertMany(context.Background(), []Value{
		{Namespace: "ns", Code: "A", ShortText: "a"},
		{Namespace: "ns", Code: "B", ShortText: "b"},
	}, "alice")
	if err != nil {
		t.Fatalf("UpsertMany: %v", err)
	}
	if len(got) != 2 || got[1].Code != "B" {
		t.Fatalf("returned = %+v", got)
	}
}

func TestUpsertManyErrors(t *testing.T) {
	svc := NewValueService(&stubRepo{}, stubSink{err: errors.New("audit down")})
	if _, err := svc.UpsertMany(context.Background(), []Value{{Namespace: "ns", Code: "A", ShortText: "x"}}, "alice"); err == nil {
		t.Fatal("expected the audit error to propagate")
	}
	// A repo failure mid-batch propagates.
	svc2 := NewValueService(&stubRepo{err: errors.New("db down")}, stubSink{})
	if _, err := svc2.UpsertMany(context.Background(), []Value{{Namespace: "ns", Code: "A", ShortText: "x"}}, "alice"); err == nil {
		t.Fatal("expected the repo error to propagate")
	}
}

func TestSeedReturnsCountAndErrors(t *testing.T) {
	svc := NewValueService(&stubRepo{}, stubSink{})
	n, err := svc.Seed(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if n != len(standardCatalogues()) {
		t.Fatalf("seeded = %d, want %d", n, len(standardCatalogues()))
	}
	// Audit failure propagates.
	svc2 := NewValueService(&stubRepo{}, stubSink{err: errors.New("audit down")})
	if _, err := svc2.Seed(context.Background(), "alice"); err == nil {
		t.Fatal("expected the audit error to propagate")
	}
	// Repo failure propagates.
	svc3 := NewValueService(&stubRepo{err: errors.New("db down")}, stubSink{})
	if _, err := svc3.Seed(context.Background(), "alice"); err == nil {
		t.Fatal("expected the repo error to propagate")
	}
}

func TestGetListDelegation(t *testing.T) {
	svc := NewValueService(&stubRepo{}, stubSink{})
	if _, err := svc.Get(context.Background(), "ns", "A"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v", err)
	}
	if _, err := svc.List(context.Background(), "ns"); err != nil {
		t.Fatalf("List = %v", err)
	}
}
