package etl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUnknownDataset is returned when a dataset name is not registered.
var ErrUnknownDataset = errors.New("etl: unknown dataset")

// Registry runs named datasets and lists the ones it holds. It is the entry point
// a runtime (the HTTP API, a worker job) uses instead of a hand-wired Dataset.
type Registry struct {
	runner   *Runner
	datasets map[string]Dataset
}

// NewRegistry builds a registry over the given datasets. A later dataset with the
// same name replaces an earlier one; a blank name is ignored.
func NewRegistry(runner *Runner, datasets ...Dataset) *Registry {
	m := make(map[string]Dataset, len(datasets))
	for _, d := range datasets {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		m[name] = d
	}
	return &Registry{runner: runner, datasets: m}
}

// Names returns the registered dataset names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.datasets))
	for name := range r.datasets {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Run runs the named dataset, or returns ErrUnknownDataset.
func (r *Registry) Run(ctx context.Context, name, by string) (Report, error) {
	ds, ok := r.datasets[strings.TrimSpace(name)]
	if !ok {
		return Report{}, fmt.Errorf("etl: %q: %w", name, ErrUnknownDataset)
	}
	return r.runner.Run(ctx, ds, by)
}
