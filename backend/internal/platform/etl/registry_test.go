package etl

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry(NewRunner(&capturedAudit{}),
		Dataset{Name: "z"},
		Dataset{Name: "a"},
		Dataset{Name: "  "}, // blank name ignored
		Dataset{Name: "a"},  // duplicate replaces
	)
	if got, want := reg.Names(), []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
}

func TestRegistryRun(t *testing.T) {
	audit := &capturedAudit{}
	runner := NewRunner(audit)
	runner.now = fixedNow
	ds := Dataset{
		Name:   "lookup",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(_ context.Context, recs []Record, _ string) (int, error) { return len(recs), nil }),
	}
	reg := NewRegistry(runner, ds)

	rep, err := reg.Run(context.Background(), "lookup", "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Dataset != "lookup" || rep.Source != 2 || rep.Loaded != 2 {
		t.Fatalf("report = %+v", rep)
	}
	// The name is trimmed.
	if _, err := reg.Run(context.Background(), "  lookup  ", "alice"); err != nil {
		t.Fatalf("trimmed name: %v", err)
	}
	// Unknown name fails closed.
	if _, err := reg.Run(context.Background(), "nope", "alice"); !errors.Is(err, ErrUnknownDataset) {
		t.Fatalf("unknown = %v, want ErrUnknownDataset", err)
	}
}
