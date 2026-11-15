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

// Retiring a mandate is a decision with a paper trail: the flag goes, the row stays, the event
// carries the state it came from — and a second revocation is refused without a second event.
func TestMandateRetireIsAuditedAndRefusedTwice(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	svc := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	ref := "RT-" + uniqueSuffix(t)
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)

	m, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "", sig, kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := svc.CreateMandate(ctx, m, "alice"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}

	retired, err := svc.RetireMandate(ctx, ref, "bob")
	if err != nil {
		t.Fatalf("RetireMandate: %v", err)
	}
	if retired.Active {
		t.Fatalf("retired = %+v, want it inactive", retired)
	}
	// The row is still readable and now says it is retired.
	again, err := svc.Mandate(ctx, ref)
	if err != nil {
		t.Fatalf("Mandate after retiring: %v", err)
	}
	if again.Active {
		t.Fatal("the stored mandate is still active after retiring it")
	}

	event, ok, err := store.LastEvent(ctx, "mandate.retire", "mandate/"+ref)
	if err != nil || !ok {
		t.Fatalf("last retire event: %v, found = %v", err, ok)
	}
	if event.Actor != "bob" || event.Before != "active=true" || event.After != "active=false" {
		t.Fatalf("audit event = %+v, want bob retiring it from active to inactive", event)
	}

	// A second revocation is a conflict, and it leaves no second event.
	if _, err := svc.RetireMandate(ctx, ref, "bob"); !errors.Is(err, kasse.ErrMandateRetired) {
		t.Fatalf("second retire = %v, want ErrMandateRetired", err)
	}
	if last, ok, err := store.LastEvent(ctx, "mandate.retire", "mandate/"+ref); err != nil || !ok || last.Seq != event.Seq {
		t.Fatalf("a refused revocation wrote an event: %+v (err = %v)", last, err)
	}
	// An unknown reference is a not-found, and a blank actor is refused before any read.
	if _, err := svc.RetireMandate(ctx, ref+"-absent", "bob"); !errors.Is(err, kasse.ErrMandateNotFound) {
		t.Fatalf("unknown reference = %v, want ErrMandateNotFound", err)
	}
	if _, err := svc.RetireMandate(ctx, ref, "  "); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("blank actor = %v, want ErrActorRequired", err)
	}
	// A retired mandate no longer appears among the active ones.
	active, err := svc.Mandates(ctx, true, "", 50, 0)
	if err != nil {
		t.Fatalf("Mandates(active): %v", err)
	}
	for _, candidate := range active {
		if candidate.Reference == ref {
			t.Fatal("the retired mandate is still listed as active")
		}
	}
	if err := store.Verify(ctx); err != nil {
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

	all, err := svc.Mandates(ctx, false, "", 50, 0)
	if err != nil {
		t.Fatalf("Mandates(all): %v", err)
	}
	eq("all", mine(all), live, retired, oneoff)

	active, err := svc.Mandates(ctx, true, "", 50, 0)
	if err != nil {
		t.Fatalf("Mandates(active): %v", err)
	}
	eq("active only", mine(active), live, oneoff)

	recurring, err := svc.Mandates(ctx, true, kasse.SequenceRecurring, 50, 0)
	if err != nil {
		t.Fatalf("Mandates(active, recurring): %v", err)
	}
	eq("active AND recurring", mine(recurring), live)

	oneoffs, err := svc.Mandates(ctx, false, kasse.SequenceOneOff, 50, 0)
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

// TestMandateListPages: the list answers the page that was asked for, and walking the pages
// reproduces a window of it in order, with nothing repeated and nothing skipped. A page past the end
// is empty rather than an error — a screen that asks once too often gets nothing, not a failure it
// would have to explain.
func TestMandateListPages(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewMandateService(kasse.NewMandateRepo(pool), audit.NewStore(pool))

	// A handful of this test's own, so the list is longer than one small page whatever else lives in
	// the shared schema. Their references share a prefix, which also sorts them predictably.
	prefix := "PG-" + uniqueSuffix(t)
	for _, suffix := range []string{"-1", "-2", "-3", "-4", "-5"} {
		m, err := kasse.NewMandate(prefix+suffix, "Max Mustermann", uniqueIBAN(t), "",
			time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC), kasse.SequenceRecurring)
		if err != nil {
			t.Fatalf("NewMandate: %v", err)
		}
		if _, err := svc.CreateMandate(ctx, m, "clerk"); err != nil {
			t.Fatalf("CreateMandate: %v", err)
		}
	}

	// A window read in one go is the reference the pages must reproduce: the same rows in the same
	// order. Reading a window rather than assuming the table is empty keeps this independent of what
	// the other tests in this package have already stored.
	const window = 4
	head, err := svc.Mandates(ctx, false, "", window, 0)
	if err != nil {
		t.Fatalf("Mandates: %v", err)
	}
	if len(head) < 2 {
		t.Fatalf("the list holds %d rows, so nothing about paging is being tested", len(head))
	}

	// The same window taken as two pages: the first from the beginning, the second from where the
	// first ended. A list that ignored the offset would repeat the first page; one that ignored the
	// limit would answer with more than was asked for.
	const pageSize = 2
	first, err := svc.Mandates(ctx, false, "", pageSize, 0)
	if err != nil {
		t.Fatalf("Mandates(offset=0): %v", err)
	}
	if len(first) > pageSize {
		t.Fatalf("a page of %d holds %d rows", pageSize, len(first))
	}
	second, err := svc.Mandates(ctx, false, "", pageSize, pageSize)
	if err != nil {
		t.Fatalf("Mandates(offset=%d): %v", pageSize, err)
	}
	if len(second) > pageSize {
		t.Fatalf("a page of %d holds %d rows", pageSize, len(second))
	}

	paged := append(append([]kasse.Mandate{}, first...), second...)
	if len(paged) != len(head) {
		t.Fatalf("two pages of %d yielded %d rows, want the %d of one window", pageSize, len(paged), len(head))
	}
	for i := range head {
		if paged[i].Reference != head[i].Reference {
			t.Fatalf("the pages diverge from the window at %d: got %q, want %q — the order is not stable",
				i, paged[i].Reference, head[i].Reference)
		}
	}

	// The filter travels with the page: a page of this test's recurring mandates holds recurring ones
	// only, and a walk of those pages finds every one this test stored.
	var seen int
	for offset := 0; ; offset += pageSize {
		page, err := svc.Mandates(ctx, false, kasse.SequenceRecurring, pageSize, offset)
		if err != nil {
			t.Fatalf("Mandates(recurring, offset=%d): %v", offset, err)
		}
		for _, m := range page {
			if m.SequenceType != kasse.SequenceRecurring {
				t.Fatalf("a page of recurring mandates holds a %s one", m.SequenceType)
			}
			if strings.HasPrefix(m.Reference, prefix) {
				seen++
			}
		}
		if len(page) < pageSize {
			break
		}
	}
	if seen != 5 {
		t.Fatalf("the walk found %d of this test's 5 mandates", seen)
	}

	// A page past the end is empty, not an error.
	past, err := svc.Mandates(ctx, false, "", 5, 100000)
	if err != nil {
		t.Fatalf("Mandates(past the end): %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows", len(past))
	}
}
