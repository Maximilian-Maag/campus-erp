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
	if len(vat) != 7 {
		t.Fatalf("vat_category = %d, want 7", len(vat))
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
