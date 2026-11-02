package ledger_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/ledger"
)

func posting(ref string) ledger.Posting {
	return ledger.Posting{Reference: ref, BookingDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Account: "8400", Direction: ledger.Credit, AmountCents: 100}
}

func TestReconcileBalanced(t *testing.T) {
	r := ledger.Reconcile([]string{"BK-1", "BK-2"}, []ledger.Posting{posting("BK-2"), posting("BK-1")})
	if !r.Balanced() || r.Matched != 2 {
		t.Fatalf("reconciliation = %+v", r)
	}
	if r.Missing != nil || r.Extra != nil {
		t.Fatalf("nothing should be unmatched: %+v", r)
	}
}

func TestReconcileFindsUnmatched(t *testing.T) {
	r := ledger.Reconcile([]string{"BK-1", "BK-2", "BK-3"}, []ledger.Posting{posting("BK-1"), posting("BK-9")})
	if r.Balanced() {
		t.Fatal("expected an imbalance")
	}
	if r.Matched != 1 {
		t.Fatalf("matched = %d, want 1", r.Matched)
	}
	if !reflect.DeepEqual(r.Missing, []string{"BK-2", "BK-3"}) {
		t.Fatalf("missing = %v", r.Missing)
	}
	if !reflect.DeepEqual(r.Extra, []string{"BK-9"}) {
		t.Fatalf("extra = %v", r.Extra)
	}
}

func TestReconcileDetectsDuplicates(t *testing.T) {
	// The same booking exported twice is caught: the second posting is extra and the
	// second booking (had it existed) would be missing.
	r := ledger.Reconcile([]string{"BK-1", "BK-1"}, []ledger.Posting{posting("BK-1"), posting("BK-1"), posting("BK-1")})
	if r.Matched != 2 || !reflect.DeepEqual(r.Extra, []string{"BK-1"}) {
		t.Fatalf("reconciliation = %+v", r)
	}
	r = ledger.Reconcile([]string{"BK-1", "BK-1"}, []ledger.Posting{posting("BK-1")})
	if !reflect.DeepEqual(r.Missing, []string{"BK-1"}) || r.Extra != nil {
		t.Fatalf("reconciliation = %+v", r)
	}
}

func TestReconcileEmpty(t *testing.T) {
	r := ledger.Reconcile(nil, nil)
	if !r.Balanced() || r.Matched != 0 || len(r.Missing) != 0 || len(r.Extra) != 0 {
		t.Fatalf("empty reconciliation = %+v", r)
	}
}

func TestReconcileTrimsReferences(t *testing.T) {
	r := ledger.Reconcile([]string{" BK-1 "}, []ledger.Posting{{Reference: "BK-1"}})
	if !r.Balanced() || r.Matched != 1 {
		t.Fatalf("references must be trimmed: %+v", r)
	}
}
