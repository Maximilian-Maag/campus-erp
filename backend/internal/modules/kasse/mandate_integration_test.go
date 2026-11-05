package kasse_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func TestMandatePersistsAndIsAudited(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewMandateService(kasse.NewMandateRepo(pool), audit.NewStore(pool))
	ref := "MAN-" + uniqueSuffix(t)
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)

	m, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "COBADEFFXXX", sig, kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	saved, err := svc.CreateMandate(ctx, m, "alice")
	if err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	if saved.ID == 0 || !saved.Active {
		t.Fatalf("saved = %+v", saved)
	}

	got, err := svc.Mandate(ctx, ref)
	if err != nil {
		t.Fatalf("Mandate: %v", err)
	}
	if got.DebtorName != "Max Mustermann" || got.SequenceType != kasse.SequenceRecurring ||
		got.DebtorBIC != "COBADEFFXXX" || !got.SignatureDate.Equal(sig) {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// A duplicate reference is a conflict.
	if _, err := svc.CreateMandate(ctx, m, "alice"); !errors.Is(err, kasse.ErrMandateReferenceExists) {
		t.Fatalf("duplicate = %v, want ErrMandateReferenceExists", err)
	}
	// A missing reference is not found.
	if _, err := svc.Mandate(ctx, ref+"-absent"); !errors.Is(err, kasse.ErrMandateNotFound) {
		t.Fatalf("missing = %v, want ErrMandateNotFound", err)
	}
	// A blank actor is rejected before any write.
	if _, err := svc.CreateMandate(ctx, kasse.Mandate{Reference: ref + "-x"}, "  "); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("actor = %v, want ErrActorRequired", err)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestMandatesListFilters(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewMandateService(kasse.NewMandateRepo(pool), audit.NewStore(pool))
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)

	// Three mandates of this test's own, so the shared schema's other rows cannot
	// decide the result; a retired recurring one and a live one-off.
	prefix := "LM-" + uniqueSuffix(t)
	mk := func(ref string, seq kasse.MandateSequence) {
		m, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "", sig, seq)
		if err != nil {
			t.Fatalf("NewMandate(%s): %v", ref, err)
		}
		if _, err := svc.CreateMandate(ctx, m, "alice"); err != nil {
			t.Fatalf("CreateMandate(%s): %v", ref, err)
		}
	}
	live := prefix + "-A"
	retired := prefix + "-R"
	oneoff := prefix + "-O"
	mk(live, kasse.SequenceRecurring)
	mk(retired, kasse.SequenceRecurring)
	mk(oneoff, kasse.SequenceOneOff)
	if _, err := pool.Exec(ctx, `UPDATE direct_debit_mandate SET active = false WHERE mandate_reference = $1`, retired); err != nil {
		t.Fatalf("retire: %v", err)
	}

	// mine narrows a list to this test's own references, so the assert is about the
	// filter and not about whatever else lives in the schema.
	mine := func(ms []kasse.Mandate) map[string]bool {
		out := map[string]bool{}
		for _, m := range ms {
			if strings.HasPrefix(m.Reference, prefix) {
				out[m.Reference] = true
			}
		}
		return out
	}
	eq := func(name string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for _, r := range want {
			if !got[r] {
				t.Fatalf("%s: %v missing %q", name, got, r)
			}
		}
	}

	all, err := svc.Mandates(ctx, false, "")
	if err != nil {
		t.Fatalf("Mandates(all): %v", err)
	}
	eq("all", mine(all), live, retired, oneoff)

	active, err := svc.Mandates(ctx, true, "")
	if err != nil {
		t.Fatalf("Mandates(active): %v", err)
	}
	eq("active only", mine(active), live, oneoff)

	recurring, err := svc.Mandates(ctx, true, kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("Mandates(active, recurring): %v", err)
	}
	eq("active AND recurring", mine(recurring), live)

	oneoffs, err := svc.Mandates(ctx, false, kasse.SequenceOneOff)
	if err != nil {
		t.Fatalf("Mandates(one-off): %v", err)
	}
	eq("one-off regardless of state", mine(oneoffs), oneoff)

	// The list is by reference, so two calls agree and a screen shows a stable order.
	refs := make([]string, 0, len(all))
	for _, m := range all {
		refs = append(refs, m.Reference)
	}
	if !sort.StringsAreSorted(refs) {
		t.Fatalf("mandates not ordered by reference: %v", refs)
	}
}
