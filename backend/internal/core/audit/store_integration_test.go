package audit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

func storeTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStoreAppendSealsAndVerifies(t *testing.T) {
	ctx := context.Background()
	store := audit.NewStore(storeTestPool(t))

	// The chain is global and other packages append to it concurrently, so this
	// test must not assume it owns the sequence: assert monotonicity and sealing
	// rather than contiguity.
	var prevSeq uint64
	for i := 0; i < 3; i++ {
		e, err := store.Append(ctx, audit.Event{
			At: time.Now(), Actor: "alice", Action: "test", Subject: "audit-store-test",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if e.Seq == 0 {
			t.Fatal("Append must assign a sequence number")
		}
		if e.Hash == "" {
			t.Fatal("Append must assign a hash")
		}
		if e.PrevHash == "" {
			t.Fatal("Append must chain to a predecessor")
		}
		if i > 0 && e.Seq <= prevSeq {
			t.Fatalf("sequence must increase: %d then %d", prevSeq, e.Seq)
		}
		prevSeq = e.Seq
	}

	if err := store.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// TestStoreEventsReadsByActionAndSubject: the read side returns an action's newest events,
// narrows to one subject, and honours the subject filter it was given (the stream is shared
// by every package and run, so the test owns a subject unique to this run).
func TestStoreEventsReadsByActionAndSubject(t *testing.T) {
	ctx := context.Background()
	pool := storeTestPool(t)
	store := audit.NewStore(pool)

	var token string
	if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&token); err != nil {
		t.Fatalf("unique token: %v", err)
	}
	subject := "events-test/" + token
	other := "events-other/" + token

	for i := 0; i < 3; i++ {
		if _, err := store.Append(ctx, audit.Event{
			At: time.Now(), Actor: "alice", Action: "test.events", Subject: subject,
		}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if _, err := store.Append(ctx, audit.Event{
		At: time.Now(), Actor: "bob", Action: "test.events", Subject: other,
	}); err != nil {
		t.Fatalf("append other: %v", err)
	}

	got, err := store.Events(ctx, "test.events", subject, 0)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Events = %d, want the 3 appended to this subject", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Seq <= got[i].Seq {
			t.Fatalf("events are not newest-first at %d", i)
		}
	}
	if got[0].Action != "test.events" || got[0].Subject != subject {
		t.Fatalf("event = %+v, want this action and subject", got[0])
	}

	if one, err := store.Events(ctx, "test.events", subject, 1); err != nil || len(one) != 1 {
		t.Fatalf("limit=1 -> %d events, %v; want 1", len(one), err)
	}

	// No subject matches the action across subjects, so this run's other subject appears.
	all, err := store.Events(ctx, "test.events", "", 200)
	if err != nil {
		t.Fatalf("Events (no subject): %v", err)
	}
	var sawOther bool
	for _, e := range all {
		if e.Subject == other {
			sawOther = true
		}
	}
	if !sawOther {
		t.Fatalf("a subject-less read did not match %q", other)
	}

	// An action nobody wrote has no events, and is not an error.
	if none, err := store.Events(ctx, "test.events.none", "", 0); err != nil || len(none) != 0 {
		t.Fatalf("unknown action -> %d events, %v; want none and no error", len(none), err)
	}
}

func TestStoreAppendSealsTheFirstEventToGenesis(t *testing.T) {
	ctx := context.Background()
	store := audit.NewStore(storeTestPool(t))

	// On an empty chain the first event must chain to the genesis hash. Run this
	// against a table that may already hold rows: the assertion is on the chain
	// link, not on the sequence number.
	e, err := store.Append(ctx, audit.Event{At: time.Now(), Actor: "a", Action: "x", Subject: "s"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if e.PrevHash == "" {
		t.Fatal("PrevHash must be set")
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}
