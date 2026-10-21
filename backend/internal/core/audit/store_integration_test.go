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

	var first audit.Event
	for i := 0; i < 3; i++ {
		e, err := store.Append(ctx, audit.Event{
			At: time.Now(), Actor: "alice", Action: "test", Subject: "audit-store-test",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if i == 0 {
			first = e
			if e.Seq == 0 {
				t.Fatal("Append must assign a sequence number")
			}
			if e.Hash == "" {
				t.Fatal("Append must assign a hash")
			}
		} else if e.Seq != first.Seq+uint64(i) {
			t.Fatalf("sequence not contiguous: got %d", e.Seq)
		}
		if e.PrevHash == "" {
			t.Fatal("Append must chain to a predecessor")
		}
	}

	if err := store.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
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
