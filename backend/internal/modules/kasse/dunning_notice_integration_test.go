package kasse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestDunningNoticeRoundTrip covers the read path against a real database: a notice is
// inserted, its document key recorded, and both read back by id. The document route and
// any consumer depend on this.
func TestDunningNoticeRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := kasse.NewDunningRepo(testPool(t))

	inserted, err := repo.InsertNotice(ctx, kasse.DunningNotice{
		ClaimReference:   "CLAIM-ROUNDTRIP",
		LevelCode:        "2",
		OutstandingCents: 125000,
		ChargeCents:      500,
		IssuedOn:         time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		IssuedBy:         "setup",
	})
	if err != nil {
		t.Fatalf("InsertNotice: %v", err)
	}

	// Freshly inserted notices carry no document yet.
	got, err := repo.GetNotice(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetNotice: %v", err)
	}
	if got.DocumentKey != "" || got.ClaimReference != "CLAIM-ROUNDTRIP" || got.ChargeCents != 500 {
		t.Fatalf("notice = %+v", got)
	}

	if err := repo.SetNoticeDocument(ctx, inserted.ID, "dunning/roundtrip.pdf"); err != nil {
		t.Fatalf("SetNoticeDocument: %v", err)
	}
	got, err = repo.GetNotice(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetNotice after set: %v", err)
	}
	if got.DocumentKey != "dunning/roundtrip.pdf" {
		t.Fatalf("document key = %q", got.DocumentKey)
	}

	if _, err := repo.GetNotice(ctx, inserted.ID+1_000_000); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("missing notice error = %v, want ErrNotFound", err)
	}
	if err := repo.SetNoticeDocument(ctx, inserted.ID+1_000_000, "x"); err == nil {
		t.Fatal("setting a document on a missing notice must fail")
	}
}
