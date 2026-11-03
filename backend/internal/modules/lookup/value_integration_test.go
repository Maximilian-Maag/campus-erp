package lookup_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.Pool(t)
}

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func newService(t *testing.T) *lookup.ValueService {
	t.Helper()
	pool := testPool(t)
	return lookup.NewValueService(lookup.NewValueRepo(pool), audit.NewStore(pool))
}

func suffix(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000000")
}

// TestSeededCataloguesMatchCoreCodeLists: the seeded VAT categories and payment means are
// exactly the lists core/einvoice validates against — same codes, same labels — so the
// catalogue an operator sees and the codes the generator accepts cannot drift apart. This is
// the guard for docs/11's "point the enums at the catalogue": pointed, and pinned.
func TestSeededCataloguesMatchCoreCodeLists(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	if _, err := svc.Seed(ctx, "alice"); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	lists := []struct {
		namespace string
		codes     []core.Code
	}{
		{"vat_category", core.VATCategories()},
		{"payment_means", core.PaymentMeansCodes()},
	}
	for _, l := range lists {
		got, err := svc.List(ctx, l.namespace)
		if err != nil {
			t.Fatalf("List %s: %v", l.namespace, err)
		}
		if len(got) != len(l.codes) {
			t.Fatalf("%s = %d values, want core's %d", l.namespace, len(got), len(l.codes))
		}
		// Index-wise, so the seeded order (the standard's order) is pinned and not just the set.
		for i, want := range l.codes {
			if got[i].Code != want.Code || got[i].ShortText != want.ShortText || got[i].LongText != want.LongText {
				t.Fatalf("%s[%d] = %+v, want %+v", l.namespace, i, got[i], want)
			}
			if got[i].SortOrder != (i+1)*10 {
				t.Fatalf("%s/%s sort_order = %d, want %d", l.namespace, want.Code, got[i].SortOrder, (i+1)*10)
			}
		}
	}
}

func TestLookupSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	n1, err := svc.Seed(ctx, "alice")
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if n1 < 30 {
		t.Fatalf("seeded = %d", n1)
	}
	if _, err := svc.Seed(ctx, "alice"); err != nil {
		t.Fatalf("re-Seed: %v", err)
	}

	vat, err := svc.List(ctx, "vat_category")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// The seeded VAT categories are core/einvoice's list, so the count is derived from it: a
	// code added there but not seeded here (or vice versa) would fail rather than drift.
	if len(vat) != len(core.VATCategories()) {
		t.Fatalf("vat_category = %d, want core's %d", len(vat), len(core.VATCategories()))
	}
	if vat[0].Code != "S" {
		t.Fatalf("first = %+v", vat[0])
	}

	s, err := svc.Get(ctx, "vat_category", "S")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.ShortText != "Regelsteuersatz" {
		t.Fatalf("value = %+v", s)
	}

	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestLookupUpsertUpdatesInPlace(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	ns := "test_ns_" + suffix(t)

	v := lookup.Value{Namespace: ns, Code: "A", ShortText: "first", SortOrder: 1, Active: true}
	if _, err := svc.Upsert(ctx, v, "alice"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	v.ShortText = "second"
	v.Active = false
	if _, err := svc.Upsert(ctx, v, "alice"); err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}
	got, err := svc.Get(ctx, ns, "A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ShortText != "second" || got.Active {
		t.Fatalf("value = %+v", got)
	}
	list, _ := svc.List(ctx, ns)
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1 (upsert must not duplicate)", len(list))
	}

	bulk := []lookup.Value{
		{Namespace: ns, Code: "B", ShortText: "bee", Active: true},
		{Namespace: ns, Code: "C", ShortText: "cee", Active: true},
	}
	if _, err := svc.UpsertMany(ctx, bulk, "alice"); err != nil {
		t.Fatalf("UpsertMany: %v", err)
	}
	list, _ = svc.List(ctx, ns)
	if len(list) != 3 {
		t.Fatalf("after bulk = %d, want 3", len(list))
	}
}

func TestLookupValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.Upsert(ctx, lookup.Value{Namespace: "ns", Code: "A", ShortText: "x"}, "  "); !errors.Is(err, lookup.ErrActorRequired) {
		t.Fatalf("actor: %v", err)
	}
	if _, err := svc.Upsert(ctx, lookup.Value{Namespace: " ", Code: "A", ShortText: "x"}, "alice"); !errors.Is(err, lookup.ErrNamespaceRequired) {
		t.Fatalf("namespace: %v", err)
	}
	if _, err := svc.UpsertMany(ctx, []lookup.Value{{Namespace: "ns", Code: "A", ShortText: "x"}}, ""); !errors.Is(err, lookup.ErrActorRequired) {
		t.Fatalf("bulk actor: %v", err)
	}
	if _, err := svc.Seed(ctx, ""); !errors.Is(err, lookup.ErrActorRequired) {
		t.Fatalf("seed actor: %v", err)
	}
	if _, err := svc.Get(ctx, "does_not_exist", "X"); !errors.Is(err, lookup.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
}
