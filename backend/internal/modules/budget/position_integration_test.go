package budget_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return dbtest.Pool(t)
}

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func newService(t *testing.T) *budget.PositionService {
	t.Helper()
	pool := testPool(t)
	return budget.NewPositionService(budget.NewPositionRepo(pool), audit.NewStore(pool))
}

func suffix(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000000")
}

// A commitment is bound once per reference: a retried approval of the same order must not bind the
// amount twice, and a release nets the reference to zero so a later retry binds it again (issue
// #168).
func TestBindingACommitmentTwiceBindsOnce(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	pos, err := svc.CreatePosition(ctx, 2026, "K-1", "Prüfung", "binding", 100000, "clerk")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	reference := "ANORD:" + suffix(t)

	first, err := svc.Post(ctx, pos.ID, budget.Commitment, 25000, reference, "clerk")
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second, err := svc.Post(ctx, pos.ID, budget.Commitment, 25000, reference, "clerk")
	if err != nil {
		t.Fatalf("second bind: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("the second bind must return the posting already there: %d vs %d", second.ID, first.ID)
	}
	bal, err := svc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 25000 {
		t.Fatalf("committed = %d, want 25000 after two binds of one reference", bal.CommittedCents)
	}

	// A release nets the reference to zero, so the next approval binds it again.
	if _, err := svc.Post(ctx, pos.ID, budget.Release, 25000, reference, "clerk"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if bal, err = svc.Balance(ctx, pos.ID); err != nil {
		t.Fatalf("Balance after release: %v", err)
	} else if bal.CommittedCents != 0 {
		t.Fatalf("committed = %d, want 0 after the release", bal.CommittedCents)
	}
	third, err := svc.Post(ctx, pos.ID, budget.Commitment, 25000, reference, "clerk")
	if err != nil {
		t.Fatalf("re-bind: %v", err)
	}
	if third.ID == first.ID {
		t.Fatalf("a released reference must bind a new posting, got the original %d", third.ID)
	}
	if bal, err = svc.Balance(ctx, pos.ID); err != nil {
		t.Fatalf("Balance after re-bind: %v", err)
	} else if bal.CommittedCents != 25000 {
		t.Fatalf("committed = %d, want 25000 after the re-bind", bal.CommittedCents)
	}
}

// TestPositionsListsEveryPositionInBudgetOrder: the Mittelübersicht's data source lists every
// position, in the order a budget is read, and picks up positions created after it was asked.
func TestPositionsListsEveryPositionInBudgetOrder(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	tag := suffix(t)

	// A position from another year, to prove the ordering is by year first.
	older, err := svc.CreatePosition(ctx, 2025, "T-"+tag, "1", "", 1000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	newer, err := svc.CreatePosition(ctx, 2026, "T-"+tag, "2", "Beschaffung", 2000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}

	all, err := svc.Positions(ctx, 200, 0)
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	var seenOlder, seenNewer int
	for i, p := range all {
		switch p.ID {
		case older.ID:
			seenOlder = i + 1
		case newer.ID:
			seenNewer = i + 1
		}
	}
	if seenOlder == 0 || seenNewer == 0 {
		t.Fatalf("Positions did not list both created positions")
	}
	if seenOlder > seenNewer {
		t.Fatalf("2025 position came after the 2026 one: %d > %d", seenOlder, seenNewer)
	}
	// The order is year, chapter, title — so within a year the chapter decides.
	for i := 1; i < len(all); i++ {
		if all[i-1].Year > all[i].Year {
			t.Fatalf("positions are not ordered by year: %d before %d", all[i-1].Year, all[i].Year)
		}
	}

	// The balance of a created position is readable, which is what the overview needs.
	bal, err := svc.Balance(ctx, newer.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.AllocatedCents != 2000 || bal.AvailableCents() != 2000 {
		t.Fatalf("balance = %+v", bal)
	}
}

// TestBudgetOverspendGuard is the acceptance test: a posting beyond the available
// amount is rejected, and a release restores availability.
func TestBudgetOverspendGuard(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	pos, err := svc.CreatePosition(ctx, 2026, "05", "T-"+suffix(t), "Beschaffung", 100000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}

	// Commit 30000, spend 20000 -> 50000 available.
	if _, err := svc.Post(ctx, pos.ID, budget.Commitment, 30000, "PO-1", "alice"); err != nil {
		t.Fatalf("commitment: %v", err)
	}
	if _, err := svc.Post(ctx, pos.ID, budget.Actual, 20000, "RE-1", "alice"); err != nil {
		t.Fatalf("actual: %v", err)
	}
	bal, err := svc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.AvailableCents() != 50000 {
		t.Fatalf("available = %d, want 50000", bal.AvailableCents())
	}

	// A commitment beyond the available amount is rejected.
	if _, err := svc.Post(ctx, pos.ID, budget.Commitment, 50001, "PO-2", "alice"); !errors.Is(err, budget.ErrBudgetExceeded) {
		t.Fatalf("overspend = %v, want ErrBudgetExceeded", err)
	}
	// An exact fit passes.
	if _, err := svc.Post(ctx, pos.ID, budget.Commitment, 50000, "PO-3", "alice"); err != nil {
		t.Fatalf("exact fit: %v", err)
	}
	// Now available is zero; anything more is rejected.
	if _, err := svc.Post(ctx, pos.ID, budget.Actual, 1, "RE-2", "alice"); !errors.Is(err, budget.ErrBudgetExceeded) {
		t.Fatalf("over after exact = %v", err)
	}
	// A release restores availability.
	if _, err := svc.Post(ctx, pos.ID, budget.Release, 50000, "PO-3-cancel", "alice"); err != nil {
		t.Fatalf("release: %v", err)
	}
	bal, _ = svc.Balance(ctx, pos.ID)
	if bal.AvailableCents() != 50000 {
		t.Fatalf("available after release = %d", bal.AvailableCents())
	}
	if _, err := svc.Post(ctx, pos.ID, budget.Actual, 50000, "RE-3", "alice"); err != nil {
		t.Fatalf("spend after release: %v", err)
	}

	// Audit chain intact.
	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestBudgetValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.CreatePosition(ctx, 2026, "05", "T", "", 1, "  "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("actor: %v", err)
	}
	if _, err := svc.CreatePosition(ctx, 2026, "  ", "T", "", 1, "alice"); !errors.Is(err, budget.ErrChapterRequired) {
		t.Fatalf("chapter: %v", err)
	}

	// Duplicate position.
	title := "DUP-" + suffix(t)
	if _, err := svc.CreatePosition(ctx, 2026, "05", title, "", 1000, "alice"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreatePosition(ctx, 2026, "05", title, "", 1000, "alice"); !errors.Is(err, budget.ErrPositionExists) {
		t.Fatalf("duplicate: %v", err)
	}

	// Unknown position.
	if _, err := svc.Post(ctx, 9223372036854775807, budget.Commitment, 100, "r", "alice"); !errors.Is(err, budget.ErrPositionNotFound) {
		t.Fatalf("unknown position: %v", err)
	}
	if _, err := svc.Post(ctx, 1, budget.Commitment, 100, "r", "  "); !errors.Is(err, budget.ErrActorRequired) {
		t.Fatalf("post actor: %v", err)
	}
	if _, err := svc.GetPosition(ctx, 9223372036854775807); !errors.Is(err, budget.ErrPositionNotFound) {
		t.Fatalf("get unknown: %v", err)
	}
	if _, err := svc.Balance(ctx, 9223372036854775807); !errors.Is(err, budget.ErrPositionNotFound) {
		t.Fatalf("balance unknown: %v", err)
	}
}

// TestPositionsPages: the list answers the page it was asked for, and walking the pages reproduces a
// window of it in order, with nothing repeated and nothing skipped. The order ends in the primary key,
// which is what makes that true for positions that share a year, a chapter and a title.
func TestPositionsPages(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	tag := suffix(t)

	// Four positions of this test's own, so the window below spans several of them whatever else the
	// shared schema holds.
	for _, chapter := range []string{"A", "B", "C", "D"} {
		if _, err := svc.CreatePosition(ctx, 2026, "PG-"+tag, chapter, "Titel", 1000, "alice"); err != nil {
			t.Fatalf("CreatePosition(%s): %v", chapter, err)
		}
	}

	const window = 4
	head, err := svc.Positions(ctx, window, 0)
	if err != nil {
		t.Fatalf("Positions: %v", err)
	}
	if len(head) < 2 {
		t.Fatalf("the budget holds %d positions, so nothing about paging is being tested", len(head))
	}

	// The same window as two pages: a list that ignored the offset would repeat the first page, one
	// that ignored the limit would answer with more than was asked for.
	const pageSize = 2
	first, err := svc.Positions(ctx, pageSize, 0)
	if err != nil {
		t.Fatalf("Positions(offset=0): %v", err)
	}
	second, err := svc.Positions(ctx, pageSize, pageSize)
	if err != nil {
		t.Fatalf("Positions(offset=%d): %v", pageSize, err)
	}
	if len(first) > pageSize || len(second) > pageSize {
		t.Fatalf("a page of %d holds %d and %d rows", pageSize, len(first), len(second))
	}
	paged := append(append([]budget.Position{}, first...), second...)
	if len(paged) != len(head) {
		t.Fatalf("two pages of %d yielded %d rows, want the %d of one window", pageSize, len(paged), len(head))
	}
	for i := range head {
		if paged[i].ID != head[i].ID {
			t.Fatalf("the pages diverge from the window at %d: got %d, want %d", i, paged[i].ID, head[i].ID)
		}
	}

	// A page past the end is empty rather than an error.
	past, err := svc.Positions(ctx, 5, 100000)
	if err != nil {
		t.Fatalf("Positions(past the end): %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows", len(past))
	}
}
