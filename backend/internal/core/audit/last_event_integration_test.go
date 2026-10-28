package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

func TestStoreLastEvent(t *testing.T) {
	ctx := context.Background()
	pool := storeTestPool(t)
	store := audit.NewStore(pool)

	// The test database persists between runs, so the subject must be unique per
	// run — otherwise a previous run's events make the "no history" case flaky.
	var uid string
	if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&uid); err != nil {
		t.Fatalf("uuid: %v", err)
	}
	subject := "etl/last-event-" + uid

	// No history yet.
	if _, ok, err := store.LastEvent(ctx, "etl.run", subject); err != nil || ok {
		t.Fatalf("empty: ok=%t err=%v", ok, err)
	}

	first, err := store.Append(ctx, audit.Event{At: time.Now(), Actor: "a", Action: "etl.run", Subject: subject, After: "checksum=one"})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := store.Append(ctx, audit.Event{At: time.Now(), Actor: "a", Action: "etl.run", Subject: subject, After: "checksum=two"})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	// A different action for the same subject must not be picked up.
	if _, err := store.Append(ctx, audit.Event{At: time.Now(), Actor: "a", Action: "other", Subject: subject, After: "checksum=ignored"}); err != nil {
		t.Fatalf("append other: %v", err)
	}

	got, ok, err := store.LastEvent(ctx, "etl.run", subject)
	if err != nil || !ok {
		t.Fatalf("last: ok=%t err=%v", ok, err)
	}
	if got.Seq != second.Seq || got.Hash != second.Hash || got.Seq <= first.Seq {
		t.Fatalf("last = seq %d, want the most recent etl.run (%d)", got.Seq, second.Seq)
	}

	if _, ok, err := store.LastEvent(ctx, "etl.run", subject+"-absent"); err != nil || ok {
		t.Fatalf("absent: ok=%t err=%v", ok, err)
	}
}
