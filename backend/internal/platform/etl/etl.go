// Package etl is the re-runnable, reconcilable pipeline that moves legacy rows
// into the canonical model (ADR-0001, roadmap S8).
//
// A pipeline is a Dataset: a named Source (the legacy side) bound to a Sink (the
// canonical side). Runs are idempotent — the Sink upserts by natural key, so
// re-running over an unchanged source converges on the same state — and every run
// leaves a Report in the audit stream. The report's Checksum is the reconciliation
// handle: two runs over the same source must produce the same checksum, and a run
// whose loaded count does not match its source count fails as a reconciliation
// error rather than silently dropping rows.
package etl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

var (
	// ErrActorRequired is returned when a run has no actor to attribute it to.
	ErrActorRequired = errors.New("etl: actor required")
	// ErrReconcile is returned when a run's loaded count does not match its source.
	ErrReconcile = errors.New("etl: reconciliation failed")
)

// Record is one extracted row: a natural Key (stable across runs) plus the
// canonical fields it maps to.
type Record struct {
	Key    string
	Fields map[string]string
}

// canonical renders the record deterministically for checksumming: the key, then
// the fields in sorted order, so field-map iteration order cannot affect it.
func (r Record) canonical() string {
	keys := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(r.Key)
	for _, k := range keys {
		b.WriteString("\x1f")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(r.Fields[k])
	}
	return b.String()
}

// Source extracts the legacy rows of a dataset.
type Source interface {
	Extract(ctx context.Context) ([]Record, error)
}

// Sink loads records into the canonical store. It must be idempotent (upsert by
// natural key) and must not partially apply a batch: either every record is
// written or none is. It returns the number of records written.
type Sink interface {
	Load(ctx context.Context, records []Record, by string) (int, error)
}

// Dataset binds a named legacy source to its canonical sink.
type Dataset struct {
	Name   string
	Source Source
	Sink   Sink
}

// Report is the outcome of one run: the reconciliation record. Source is the rows
// extracted, Loaded the rows written, Checksum a stable digest of the extracted
// set (independent of extract order) so consecutive runs can be compared.
type Report struct {
	Dataset  string    `json:"dataset"`
	Source   int       `json:"source"`
	Loaded   int       `json:"loaded"`
	Checksum string    `json:"checksum"`
	At       time.Time `json:"at"`
}

// AuditSink records each run.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// Runner runs datasets and records every run in the audit stream.
type Runner struct {
	audit AuditSink
	now   func() time.Time
}

// NewRunner wires a runner to the audit stream.
func NewRunner(sink AuditSink) *Runner {
	return &Runner{audit: sink, now: time.Now}
}

// Run extracts, loads and reconciles one dataset, then records the run.
//
// It is idempotent: the Sink upserts, so a second run over an unchanged source
// yields the same state (and the same Checksum). A run whose loaded count does
// not match the source count returns ErrReconcile — the report is still returned
// so the drift is visible.
func (r *Runner) Run(ctx context.Context, ds Dataset, by string) (Report, error) {
	if strings.TrimSpace(by) == "" {
		return Report{}, ErrActorRequired
	}
	records, err := ds.Source.Extract(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("etl: %s: extract: %w", ds.Name, err)
	}
	rep := Report{Dataset: ds.Name, Source: len(records), Checksum: Checksum(records), At: r.now().UTC()}

	loaded, err := ds.Sink.Load(ctx, records, by)
	if err != nil {
		return rep, fmt.Errorf("etl: %s: load: %w", ds.Name, err)
	}
	rep.Loaded = loaded
	if loaded != len(records) {
		return rep, fmt.Errorf("%w: %s loaded %d of %d", ErrReconcile, ds.Name, loaded, len(records))
	}
	if _, err := r.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "etl.run",
		Subject: "etl/" + ds.Name,
		After:   fmt.Sprintf("source=%d loaded=%d checksum=%s", rep.Source, rep.Loaded, rep.Checksum),
	}); err != nil {
		return rep, err
	}
	return rep, nil
}

// Checksum is a stable digest of a record set, independent of extract order.
func Checksum(records []Record) string {
	lines := make([]string, len(records))
	for i, rec := range records {
		lines[i] = rec.canonical()
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, line := range lines {
		_, _ = io.WriteString(h, line+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SourceFunc adapts a function to a Source.
type SourceFunc func(ctx context.Context) ([]Record, error)

// Extract implements Source.
func (f SourceFunc) Extract(ctx context.Context) ([]Record, error) { return f(ctx) }

// SinkFunc adapts a function to a Sink.
type SinkFunc func(ctx context.Context, records []Record, by string) (int, error)

// Load implements Sink.
func (f SinkFunc) Load(ctx context.Context, records []Record, by string) (int, error) {
	return f(ctx, records, by)
}
