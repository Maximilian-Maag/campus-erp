package kasse_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

// testPool opens the tenant database and applies migrations. In CI DATABASE_URL is
// mandatory; locally the test skips (loudly) when it is absent.
//
// Tests share one database and must not truncate shared tables (Go runs different
// packages' tests in parallel). Each test uses unique data instead.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return pool
}

// testSeqNext returns the next value of a test-only sequence, created on demand.
// The database persists between runs and is shared by test packages that Go runs in
// parallel, so uniqueness must come from the database itself: a timestamp can
// collide and a "find the first free value" scan races.
//
// CREATE SEQUENCE IF NOT EXISTS is not atomic, so parallel packages can race on it;
// the sequence merely existing is all we need, so the error is ignored.
func testSeqNext(t *testing.T) int64 {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "CREATE SEQUENCE IF NOT EXISTS test_unique_seq")
	var n int64
	if err := pool.QueryRow(ctx, "SELECT nextval('test_unique_seq')").Scan(&n); err != nil {
		t.Fatalf("test sequence: %v", err)
	}
	return n
}

// uniqueSuffix is a short, database-unique token (fits the legacy 8-char code
// column) for building codes and numbers in tests.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	return strconv.FormatInt(testSeqNext(t), 36)
}

// uniqueDate returns a database-unique business date. The base year is far beyond
// anything the suite uses, so a freshly created sequence cannot collide with rows
// left behind by earlier runs.
func uniqueDate(t *testing.T) time.Time {
	t.Helper()
	base := time.Date(5000, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, int(testSeqNext(t)))
}
func countSubject(t *testing.T, pool *pgxpool.Pool, subject string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_event WHERE subject = $1", subject).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func TestCashBookLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	svc := kasse.NewService(kasse.NewRepo(pool), store)

	day, err := svc.OpenDay(ctx, uniqueDate(t), 10000, "alice")
	if err != nil {
		t.Fatalf("OpenDay: %v", err)
	}
	if _, err := svc.Book(ctx, day.ID, kasse.Income, 500, "Gebühr", "alice"); err != nil {
		t.Fatalf("Book income: %v", err)
	}
	if _, err := svc.Book(ctx, day.ID, kasse.Expense, 200, "Auszahlung", "alice"); err != nil {
		t.Fatalf("Book expense: %v", err)
	}

	closed, err := svc.CloseDay(ctx, day.ID, "bob")
	if err != nil {
		t.Fatalf("CloseDay: %v", err)
	}
	if closed.Status != kasse.DayClosed {
		t.Fatalf("status = %q, want closed", closed.Status)
	}
	if closed.ClosingCents == nil || *closed.ClosingCents != 10300 {
		t.Fatalf("closing = %v, want 10300", closed.ClosingCents)
	}

	// A closed day rejects further bookings.
	if _, err := svc.Book(ctx, day.ID, kasse.Income, 1, "x", "alice"); !errors.Is(err, kasse.ErrDayClosed) {
		t.Fatalf("want ErrDayClosed, got %v", err)
	}

	// The audit chain must be intact: open + 2 bookings + close = 4 new events.
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
	subject := fmt.Sprintf("day/%d", day.ID)
	if got := countSubject(t, pool, subject); got != 4 {
		t.Fatalf("audit events for %s = %d, want 4", subject, got)
	}
}

func TestUnknownDayIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewService(kasse.NewRepo(pool), audit.NewStore(pool))
	if _, err := svc.Day(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAuditStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	// Many appends must survive the PostgreSQL round-trip and still verify. A
	// nanosecond-precision timestamp would fail here: timestamptz stores
	// microseconds, so the hash recomputed after reading back would differ.
	//
	// Tamper detection is covered by the in-memory unit tests; it is deliberately
	// NOT tested against the shared table, because mutating the chain would break
	// Verify for tests running concurrently in other packages.
	for i := 0; i < 25; i++ {
		if _, err := store.Append(ctx, audit.Event{
			At: time.Now(), Actor: "alice", Action: "test", Subject: "s", After: "x",
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("chain must verify after round-trip: %v", err)
	}
}

func TestOpenDayRejectsDuplicateBusinessDate(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewService(kasse.NewRepo(pool), audit.NewStore(pool))

	date := uniqueDate(t)
	if _, err := svc.OpenDay(ctx, date, 0, "alice"); err != nil {
		t.Fatalf("first OpenDay: %v", err)
	}
	if _, err := svc.OpenDay(ctx, date, 0, "alice"); !errors.Is(err, kasse.ErrDayExists) {
		t.Fatalf("want ErrDayExists for a duplicate business date, got %v", err)
	}
}
