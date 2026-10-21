package kasse_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func newDunningFixture(t *testing.T) (*kasse.DunningService, string) {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewDunningService(kasse.NewDunningRepo(pool), audit.NewStore(pool))

	code := "L" + uniqueSuffix(t)
	level := kasse.DunningLevel{
		Year: 2026, Code: code, Description: "Teststufe", DaysAllowed: 14, Active: true, NextCode: "2",
		Bands: []kasse.DunningBand{
			{UpToCents: 5000, FeeCents: 500, RateBasisPoints: 0},
			{UpToCents: 50000, FeeCents: 1500, RateBasisPoints: 100},
			{UpToCents: 0, FeeCents: 2500, RateBasisPoints: 200},
		},
	}
	if _, err := svc.CreateLevel(ctx, level, "alice"); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	return svc, code
}

func TestDunningLevelRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, code := newDunningFixture(t)

	reloaded, err := svc.Level(ctx, 2026, code)
	if err != nil {
		t.Fatalf("Level: %v", err)
	}
	if reloaded.Description != "Teststufe" || reloaded.DaysAllowed != 14 || !reloaded.Active {
		t.Fatalf("reloaded level = %+v", reloaded)
	}
	if len(reloaded.Bands) != 3 {
		t.Fatalf("bands = %d, want 3", len(reloaded.Bands))
	}
	// Bands must come back in their stored order.
	want := []int64{5000, 50000, 0}
	for i, b := range reloaded.Bands {
		if b.UpToCents != want[i] {
			t.Fatalf("band %d up_to = %d, want %d (order not preserved)", i, b.UpToCents, want[i])
		}
	}
	// The reloaded level must be usable: the charge depends on the bands.
	charge, err := reloaded.Charge(12000)
	if err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if charge != 1620 { // fee 1500 + 1% of 12000 = 120
		t.Fatalf("charge = %d, want 1620", charge)
	}

	if got := countSubject(t, testPool(t), fmt.Sprintf("dunning-level/2026/%s", code)); got != 1 {
		t.Fatalf("audit events for the level = %d, want 1", got)
	}
}

func TestDunningIssuePersistsCharge(t *testing.T) {
	ctx := context.Background()
	svc, code := newDunningFixture(t)

	notice, err := svc.Issue(ctx, 2026, code, "F-2026-001", 12000, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if notice.ID == 0 {
		t.Fatal("notice id not assigned")
	}
	if notice.ChargeCents != 1620 {
		t.Fatalf("charge = %d, want 1620", notice.ChargeCents)
	}
	if notice.IssuedOn.IsZero() {
		t.Fatal("issued_on not stamped")
	}

	var stored int64
	if err := testPool(t).QueryRow(ctx,
		"SELECT charge_cents FROM dunning_notice WHERE id = $1", notice.ID).Scan(&stored); err != nil {
		t.Fatalf("read notice: %v", err)
	}
	if stored != 1620 {
		t.Fatalf("stored charge = %d, want 1620", stored)
	}

	if got := countSubject(t, testPool(t), fmt.Sprintf("notice/%d", notice.ID)); got != 1 {
		t.Fatalf("audit events for the notice = %d, want 1", got)
	}
}

func TestDunningIssueRefusesUnknownAndInactiveLevels(t *testing.T) {
	ctx := context.Background()
	svc, code := newDunningFixture(t)

	if _, err := svc.Issue(ctx, 2026, "does-not-exist", "F-1", 1000, "alice"); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("unknown level: want ErrNotFound, got %v", err)
	}

	// An inactive level must not be usable for a notice.
	pool := testPool(t)
	inactiveCode := code + "x"
	inactive := kasse.DunningLevel{
		Year: 2026, Code: inactiveCode, Description: "inaktiv", Active: false,
		Bands: []kasse.DunningBand{{UpToCents: 0, FeeCents: 100, RateBasisPoints: 0}},
	}
	if _, err := kasse.NewDunningService(kasse.NewDunningRepo(pool), audit.NewStore(pool)).CreateLevel(ctx, inactive, "alice"); err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	if _, err := svc.Issue(ctx, 2026, inactiveCode, "F-1", 1000, "alice"); !errors.Is(err, kasse.ErrLevelInactive) {
		t.Fatalf("inactive level: want ErrLevelInactive, got %v", err)
	}
}

// The service validates before persisting, but the database constraints are the
// last line of defence: calling the repository directly with a negative fee must
// still fail, and the whole level must be rolled back.
func TestDunningRepoRejectsInvalidBand(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo := kasse.NewDunningRepo(pool)

	code := "L" + uniqueSuffix(t)
	_, err := repo.CreateLevel(ctx, kasse.DunningLevel{
		Year: 2026, Code: code, Description: "ungültig", Active: true,
		Bands: []kasse.DunningBand{
			{UpToCents: 100, FeeCents: 0, RateBasisPoints: 0},
			{UpToCents: 200, FeeCents: -1, RateBasisPoints: 0}, // the second band is invalid
		},
	})
	if err == nil {
		t.Fatal("a negative fee must be rejected by the database constraint")
	}
	// The error must name the band that failed, not just any band.
	if !strings.Contains(err.Error(), "band 2") {
		t.Fatalf("error should identify the failing band: %v", err)
	}

	// The failed insert must not have left a partial level behind.
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM dunning_level WHERE year = 2026 AND code = $1", code).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back level left %d rows behind", count)
	}
}

func TestDunningDuplicateLevelIsRejected(t *testing.T) {
	ctx := context.Background()
	svc, code := newDunningFixture(t)

	duplicate := kasse.DunningLevel{
		Year: 2026, Code: code, Description: "doppelt", Active: true,
		Bands: []kasse.DunningBand{{UpToCents: 0, FeeCents: 0, RateBasisPoints: 0}},
	}
	if _, err := svc.CreateLevel(ctx, duplicate, "alice"); !errors.Is(err, kasse.ErrLevelExists) {
		t.Fatalf("want ErrLevelExists, got %v", err)
	}
}
