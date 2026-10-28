package etl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

type fakeReader struct {
	event audit.Event
	ok    bool
	err   error
}

func (f fakeReader) LastEvent(context.Context, string, string) (audit.Event, bool, error) {
	return f.event, f.ok, f.err
}

type fakeLedger struct {
	checksum string
	ok       bool
	err      error
}

func (f fakeLedger) LastChecksum(context.Context, string) (string, bool, error) {
	return f.checksum, f.ok, f.err
}

func TestAfterTextRoundTrips(t *testing.T) {
	withPrev := afterText(Report{Dataset: "d", Source: 2, Loaded: 2, Checksum: "abc123", Previous: "old", Drift: true})
	if got, ok := checksumOf(withPrev); !ok || got != "abc123" {
		t.Fatalf("checksumOf = %q, %t", got, ok)
	}
	for _, want := range []string{"previous=old", "drift=true"} {
		if !strings.Contains(withPrev, want) {
			t.Fatalf("detail %q missing %q", withPrev, want)
		}
	}
	// Without a previous run only the core fields are written, and still parse.
	noPrev := afterText(Report{Source: 1, Loaded: 1, Checksum: "z"})
	if got, ok := checksumOf(noPrev); !ok || got != "z" {
		t.Fatalf("checksumOf = %q, %t", got, ok)
	}
	if strings.Contains(noPrev, "previous=") {
		t.Fatalf("detail %q should carry no previous field", noPrev)
	}
	if _, ok := checksumOf("source=1 loaded=1"); ok {
		t.Fatalf("expected no checksum in a detail without one")
	}
}

func TestAuditLedgerReadsPreviousChecksum(t *testing.T) {
	event := audit.Event{Action: "etl.run", Subject: "etl/d", After: afterText(Report{Checksum: "prev"})}
	l := NewAuditLedger(fakeReader{event: event, ok: true})
	got, ok, err := l.LastChecksum(context.Background(), "d")
	if err != nil || !ok || got != "prev" {
		t.Fatalf("got %q, %t, %v", got, ok, err)
	}
}

func TestAuditLedgerWithoutHistory(t *testing.T) {
	l := NewAuditLedger(fakeReader{ok: false})
	if _, ok, err := l.LastChecksum(context.Background(), "d"); ok || err != nil {
		t.Fatalf("ok=%t err=%v, want false/nil", ok, err)
	}
}

func TestAuditLedgerPropagatesError(t *testing.T) {
	boom := errors.New("read")
	l := NewAuditLedger(fakeReader{err: boom})
	if _, _, err := l.LastChecksum(context.Background(), "d"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestAuditLedgerIgnoresUnparseableDetail(t *testing.T) {
	l := NewAuditLedger(fakeReader{event: audit.Event{After: "no checksum here"}, ok: true})
	if _, ok, err := l.LastChecksum(context.Background(), "d"); ok || err != nil {
		t.Fatalf("ok=%t err=%v, want false/nil", ok, err)
	}
}

func driftDataset() Dataset {
	return Dataset{
		Name:   "d",
		Source: SourceFunc(func(context.Context) ([]Record, error) { return records(), nil }),
		Sink:   SinkFunc(func(_ context.Context, recs []Record, _ string) (int, error) { return len(recs), nil }),
	}
}

func TestRunReportsDrift(t *testing.T) {
	checksum := Checksum(records())
	cases := []struct {
		name     string
		ledger   Ledger
		wantPrev string
		wantDrif bool
	}{
		{"no history", fakeLedger{ok: false}, "", false},
		{"unchanged", fakeLedger{checksum: checksum, ok: true}, checksum, false},
		{"changed", fakeLedger{checksum: "stale", ok: true}, "stale", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRunner(&capturedAudit{}).WithLedger(tc.ledger)
			rep, err := r.Run(context.Background(), driftDataset(), "alice")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if rep.Previous != tc.wantPrev || rep.Drift != tc.wantDrif {
				t.Fatalf("previous=%q drift=%t, want %q/%t", rep.Previous, rep.Drift, tc.wantPrev, tc.wantDrif)
			}
		})
	}
}

func TestRunPropagatesLedgerError(t *testing.T) {
	boom := errors.New("ledger")
	r := NewRunner(&capturedAudit{}).WithLedger(fakeLedger{err: boom})
	if _, err := r.Run(context.Background(), driftDataset(), "alice"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
