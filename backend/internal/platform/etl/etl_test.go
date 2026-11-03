package etl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

type capturedAudit struct {
	events []audit.Event
	err    error
}

func (c *capturedAudit) Append(_ context.Context, e audit.Event) (audit.Event, error) {
	if c.err != nil {
		return audit.Event{}, c.err
	}
	c.events = append(c.events, e)
	return e, nil
}

// Events mimics the store's read: the action's events, newest first, optionally narrowed to
// one subject, up to limit.
func (c *capturedAudit) Events(_ context.Context, action, subject string, limit int) ([]audit.Event, error) {
	if c.err != nil {
		return nil, c.err
	}
	out := []audit.Event{}
	for i := len(c.events) - 1; i >= 0; i-- {
		e := c.events[i]
		if e.Action != action || (subject != "" && e.Subject != subject) {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}

// runOnce runs a minimal dataset under name, so a test can build up run history.
func runOnce(t *testing.T, r *Runner, name string) {
	t.Helper()
	ds := Dataset{
		Name:   name,
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(_ context.Context, recs []Record, _ string) (int, error) { return len(recs), nil }),
	}
	if _, err := r.Run(context.Background(), ds, "alice"); err != nil {
		t.Fatalf("run %s: %v", name, err)
	}
}

// TestRunnerHistoryReadsTheAuditStream: the run history is the audit record read back —
// newest first, filterable by dataset, and stamped with the run's own time.
func TestRunnerHistoryReadsTheAuditStream(t *testing.T) {
	ca := &capturedAudit{}
	r := NewRunner(ca)
	r.now = fixedNow

	runOnce(t, r, "lookup")
	runOnce(t, r, "lookup")
	runOnce(t, r, "other")

	runs, err := r.History(context.Background(), "lookup", 0)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("lookup history = %d runs, want 2", len(runs))
	}
	if runs[0].At.IsZero() || !runs[0].At.Equal(fixedNow()) {
		t.Fatalf("run time = %v, want the run's own time (the audit event was unset)", runs[0].At)
	}
	if runs[0].Actor != "alice" || runs[0].Summary == "" {
		t.Fatalf("run = %+v, want the actor and the recorded summary", runs[0])
	}
	if runs[0].Summary != runs[1].Summary {
		t.Logf("note: summaries differ (%q vs %q)", runs[0].Summary, runs[1].Summary)
	}

	// An empty dataset matches every dataset; the limit is honoured.
	all, err := r.History(context.Background(), "", 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("all history = %d runs, %v; want 3", len(all), err)
	}
	if limited, err := r.History(context.Background(), "", 1); err != nil || len(limited) != 1 {
		t.Fatalf("limited history = %d runs, %v; want 1", len(limited), err)
	}

	// A dataset that never ran has no history, and is not an error.
	if none, err := r.History(context.Background(), "never-ran", 0); err != nil || len(none) != 0 {
		t.Fatalf("unknown dataset history = %d runs, %v; want none and no error", len(none), err)
	}
}

// TestRunnerHistoryPropagatesStoreFailure: a read that cannot reach the stream is an error,
// not an empty history (which would read as "it never ran").
func TestRunnerHistoryPropagatesStoreFailure(t *testing.T) {
	r := NewRunner(&capturedAudit{err: errors.New("stream down")})
	if _, err := r.History(context.Background(), "lookup", 5); err == nil {
		t.Fatal("History = nil error, want the store failure")
	}
}

func records() []Record {
	return []Record{
		{Key: "ns/A", Fields: map[string]string{"namespace": "ns", "code": "A", "short_text": "alpha"}},
		{Key: "ns/B", Fields: map[string]string{"namespace": "ns", "code": "B", "short_text": "beta"}},
	}
}

func fixedNow() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }

func TestChecksumIsOrderIndependent(t *testing.T) {
	a := []Record{
		{Key: "k1", Fields: map[string]string{"x": "1", "y": "2"}},
		{Key: "k2", Fields: map[string]string{"x": "3"}},
	}
	b := []Record{
		{Key: "k2", Fields: map[string]string{"x": "3"}},
		{Key: "k1", Fields: map[string]string{"y": "2", "x": "1"}},
	}
	if Checksum(a) != Checksum(b) {
		t.Fatalf("checksum depends on order: %s vs %s", Checksum(a), Checksum(b))
	}
}

func TestChecksumReflectsContent(t *testing.T) {
	base := []Record{{Key: "k", Fields: map[string]string{"x": "1"}}}
	cases := []struct {
		name string
		recs []Record
	}{
		{"changed value", []Record{{Key: "k", Fields: map[string]string{"x": "2"}}}},
		{"changed key", []Record{{Key: "k2", Fields: map[string]string{"x": "1"}}}},
		{"extra field", []Record{{Key: "k", Fields: map[string]string{"x": "1", "y": ""}}}},
		{"empty", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if Checksum(base) == Checksum(tc.recs) {
				t.Fatalf("checksum unchanged for %s", tc.name)
			}
		})
	}
}

func TestRunReportsAndAudits(t *testing.T) {
	audit := &capturedAudit{}
	r := NewRunner(audit)
	r.now = fixedNow

	loaded := 0
	ds := Dataset{
		Name:   "lookup",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink: SinkFunc(func(_ context.Context, recs []Record, by string) (int, error) {
			if by != "alice" {
				t.Fatalf("by = %q", by)
			}
			loaded = len(recs)
			return len(recs), nil
		}),
	}
	rep, err := r.Run(context.Background(), ds, "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Dataset != "lookup" || rep.Source != 2 || rep.Loaded != 2 || loaded != 2 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Checksum != Checksum(records()) {
		t.Fatalf("checksum = %s", rep.Checksum)
	}
	if !rep.At.Equal(fixedNow()) {
		t.Fatalf("at = %v", rep.At)
	}
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d", len(audit.events))
	}
	e := audit.events[0]
	if e.Action != "etl.run" || e.Subject != "etl/lookup" || e.Actor != "alice" {
		t.Fatalf("event = %+v", e)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	r := NewRunner(&capturedAudit{})
	store := map[string]Record{}
	ds := Dataset{
		Name:   "lookup",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink: SinkFunc(func(_ context.Context, recs []Record, _ string) (int, error) {
			for _, rec := range recs {
				store[rec.Key] = rec
			}
			return len(recs), nil
		}),
	}
	first, err := r.Run(context.Background(), ds, "alice")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := r.Run(context.Background(), ds, "alice")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.Checksum != second.Checksum {
		t.Fatalf("checksum drifted: %s vs %s", first.Checksum, second.Checksum)
	}
	if len(store) != 2 {
		t.Fatalf("store size = %d, want 2 (upsert must not duplicate)", len(store))
	}
}

func TestRunRequiresActor(t *testing.T) {
	r := NewRunner(&capturedAudit{})
	ds := Dataset{Name: "x", Source: SourceFunc(func(context.Context) ([]Record, error) { return nil, nil })}
	if _, err := r.Run(context.Background(), ds, "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("err = %v, want ErrActorRequired", err)
	}
}

func TestRunPropagatesExtractError(t *testing.T) {
	boom := errors.New("boom")
	r := NewRunner(&capturedAudit{})
	ds := Dataset{Name: "x", Source: SourceFunc(func(context.Context) ([]Record, error) { return nil, boom })}
	if _, err := r.Run(context.Background(), ds, "alice"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestRunPropagatesLoadError(t *testing.T) {
	boom := errors.New("load")
	r := NewRunner(&capturedAudit{})
	ds := Dataset{
		Name:   "x",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(context.Context, []Record, string) (int, error) { return 0, boom }),
	}
	if _, err := r.Run(context.Background(), ds, "alice"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want load", err)
	}
}

func TestRunFailsClosedOnPartialLoad(t *testing.T) {
	audit := &capturedAudit{}
	r := NewRunner(audit)
	ds := Dataset{
		Name:   "x",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(context.Context, []Record, string) (int, error) { return 1, nil }),
	}
	rep, err := r.Run(context.Background(), ds, "alice")
	if !errors.Is(err, ErrReconcile) {
		t.Fatalf("err = %v, want ErrReconcile", err)
	}
	if rep.Source != 2 || rep.Loaded != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if len(audit.events) != 0 {
		t.Fatalf("a failed run must not be recorded as success")
	}
}

func TestRunPropagatesAuditError(t *testing.T) {
	boom := errors.New("audit down")
	r := NewRunner(&capturedAudit{err: boom})
	ds := Dataset{
		Name:   "x",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(_ context.Context, recs []Record, _ string) (int, error) { return len(recs), nil }),
	}
	if _, err := r.Run(context.Background(), ds, "alice"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want audit", err)
	}
}
