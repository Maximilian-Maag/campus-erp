package klr_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.Pool(t)
}

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func newService(t *testing.T) *klr.Service {
	t.Helper()
	pool := testPool(t)
	return klr.NewService(klr.NewRepo(pool), audit.NewStore(pool))
}

func suffix(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000000")
}

// TestKLRReportAndIsolation is the acceptance test: the report sums costs and
// revenues per cost centre, and one cost centre never sees another's records.
func TestKLRReportAndIsolation(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	a, err := svc.CreateCostCenter(ctx, "CC-A-"+suffix(t), "Bibliothek", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter A: %v", err)
	}
	b, err := svc.CreateCostCenter(ctx, "CC-B-"+suffix(t), "Labor", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter B: %v", err)
	}
	unit, err := svc.CreateCostUnit(ctx, "CU-"+suffix(t), "Lehre", "alice")
	if err != nil {
		t.Fatalf("CreateCostUnit: %v", err)
	}

	if _, err := svc.Post(ctx, a.ID, &unit.ID, klr.Cost, 30000, klrDay(2026, 4, 30), "RE-1", "alice"); err != nil {
		t.Fatalf("post cost: %v", err)
	}
	if _, err := svc.Post(ctx, a.ID, nil, klr.Revenue, 50000, klrDay(2026, 4, 30), "ER-1", "alice"); err != nil {
		t.Fatalf("post revenue: %v", err)
	}
	if _, err := svc.Post(ctx, b.ID, nil, klr.Cost, 7000, klrDay(2026, 4, 30), "RE-2", "alice"); err != nil {
		t.Fatalf("post b cost: %v", err)
	}

	ra, err := svc.Report(ctx, a.ID)
	if err != nil {
		t.Fatalf("Report A: %v", err)
	}
	if ra.CostsCents != 30000 || ra.RevenuesCents != 50000 || ra.BalanceCents() != 20000 {
		t.Fatalf("report A = %+v", ra)
	}
	rb, err := svc.Report(ctx, b.ID)
	if err != nil {
		t.Fatalf("Report B: %v", err)
	}
	if rb.CostsCents != 7000 || rb.RevenuesCents != 0 || rb.BalanceCents() != -7000 {
		t.Fatalf("report B = %+v (isolation broken)", rb)
	}

	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestKLRValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.CreateCostCenter(ctx, "1", "n", "  "); !errors.Is(err, klr.ErrActorRequired) {
		t.Fatalf("centre actor: %v", err)
	}
	if _, err := svc.CreateCostUnit(ctx, "1", "n", ""); !errors.Is(err, klr.ErrActorRequired) {
		t.Fatalf("unit actor: %v", err)
	}
	num := "CC-DUP-" + suffix(t)
	if _, err := svc.CreateCostCenter(ctx, num, "n", "alice"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateCostCenter(ctx, num, "n", "alice"); !errors.Is(err, klr.ErrCostCenterExists) {
		t.Fatalf("duplicate centre: %v", err)
	}
	unum := "CU-DUP-" + suffix(t)
	if _, err := svc.CreateCostUnit(ctx, unum, "n", "alice"); err != nil {
		t.Fatalf("create unit: %v", err)
	}
	if _, err := svc.CreateCostUnit(ctx, unum, "n", "alice"); !errors.Is(err, klr.ErrCostUnitExists) {
		t.Fatalf("duplicate unit: %v", err)
	}
	// Posting to an unknown cost centre.
	if _, err := svc.Post(ctx, 9223372036854775807, nil, klr.Cost, 100, klrDay(2026, 4, 30), "r", "alice"); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("unknown centre: %v", err)
	}
	if _, err := svc.Post(ctx, 1, nil, klr.Cost, 100, klrDay(2026, 4, 30), "r", "  "); !errors.Is(err, klr.ErrActorRequired) {
		t.Fatalf("post actor: %v", err)
	}
	if _, err := svc.GetCostCenter(ctx, 9223372036854775807); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("get unknown: %v", err)
	}
	if _, err := svc.Report(ctx, 9223372036854775807); !errors.Is(err, klr.ErrCostCenterNotFound) {
		t.Fatalf("report unknown: %v", err)
	}
}
