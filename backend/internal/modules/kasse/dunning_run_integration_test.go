package kasse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// uniqueRunYear returns a year no other test in this process configures. The
// database is isolated per test process, but a run walks the *whole* year's levels,
// so every test that builds a chain must own its year or the chains merge.
func uniqueRunYear(t *testing.T) int {
	t.Helper()
	return 2000 + int(testSeqNext(t)%900)
}

// newRunFixture creates a two-level chain for a year of its own and one open
// receivable due the year before, returning the run service, the receivable and
// the year its chain lives in.
func newRunFixture(t *testing.T) (*kasse.DunningRunService, kasse.Receivable, int) {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	dunning := kasse.NewDunningService(kasse.NewDunningRepo(pool), store)
	year := uniqueRunYear(t)

	first := "A" + uniqueSuffix(t)
	second := "B" + uniqueSuffix(t)
	mk := func(code, next string) kasse.DunningLevel {
		return kasse.DunningLevel{
			Year: year, Code: code, Description: "Stufe " + code, DaysAllowed: 0,
			Active: true, NextCode: next,
			Bands: []kasse.DunningBand{{UpToCents: 0, FeeCents: 500, RateBasisPoints: 0}},
		}
	}
	if _, err := dunning.CreateLevel(ctx, mk(first, second), "clerk"); err != nil {
		t.Fatalf("CreateLevel(%s): %v", first, err)
	}
	if _, err := dunning.CreateLevel(ctx, mk(second, ""), "clerk"); err != nil {
		t.Fatalf("CreateLevel(%s): %v", second, err)
	}

	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	mandateRef := "MAN-" + uniqueSuffix(t)
	mandate, err := kasse.NewMandate(mandateRef, "Max Mustermann", uniqueIBAN(t), "COBADEFFXXX",
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := mandates.CreateMandate(ctx, mandate, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	receivables := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	due := time.Date(year-1, 12, 31, 0, 0, 0, 0, time.UTC)
	rec, err := receivables.Post(ctx, mandateRef, 10000, "EUR", due, "Semesterbeitrag", "manual", "clerk")
	if err != nil {
		t.Fatalf("Post receivable: %v", err)
	}
	// Other tests in this process select every open, due item (a collection run does),
	// so this fixture's item must leave the open set when the test ends.
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			"UPDATE receivable SET state = 'cancelled' WHERE id = $1", rec.ID); err != nil {
			t.Logf("cancel fixture receivable %d: %v", rec.ID, err)
		}
	})

	run := kasse.NewDunningRunService(
		kasse.NewDunningRepo(pool), kasse.NewDunningRepo(pool),
		kasse.NewReceivableRepo(pool), dunning, store)
	return run, rec, year
}

func outcomeFor(t *testing.T, result kasse.DunningRunResult, claim string) kasse.DunningRunOutcome {
	t.Helper()
	for _, o := range result.Outcomes {
		if o.ClaimReference == claim {
			return o
		}
	}
	t.Fatalf("no outcome for claim %q in %+v", claim, result.Outcomes)
	return kasse.DunningRunOutcome{}
}

func TestDunningRunIssuesAndThenSkipsTheSameDay(t *testing.T) {
	ctx := context.Background()
	run, rec, year := newRunFixture(t)
	asOf := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	claim := kasse.ReceivableClaimReference(rec.ID)

	first, err := run.Run(ctx, year, asOf, "clerk")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := outcomeFor(t, first, claim)
	if !got.Issued {
		t.Fatalf("outcome = %+v, want an issued notice", got)
	}
	if got.LevelCode == "" || got.NoticeID == 0 || got.ChargeCents != 500 {
		t.Fatalf("outcome = %+v", got)
	}
	if got.DueDate.Format(time.DateOnly) != time.Date(year-1, 12, 31, 0, 0, 0, 0, time.UTC).Format(time.DateOnly) {
		t.Fatalf("due date = %v", got.DueDate)
	}

	// The notice exists, carries the computed charge, and is tied to the claim.
	var levelCode string
	var charge int64
	if err := testPool(t).QueryRow(ctx,
		"SELECT level_code, charge_cents FROM dunning_notice WHERE id = $1", got.NoticeID).
		Scan(&levelCode, &charge); err != nil {
		t.Fatalf("read notice: %v", err)
	}
	if levelCode != got.LevelCode || charge != 500 {
		t.Fatalf("stored notice = %s/%d", levelCode, charge)
	}

	// The run is audited once.
	subject := "dunning-run/" + time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006/2006-01-02")
	if n := countSubject(t, testPool(t), subject); n != 1 {
		t.Fatalf("audit events for %s = %d, want 1", subject, n)
	}

	// The same day again must not issue a second notice for the claim.
	second, err := run.Run(ctx, year, asOf, "clerk")
	if err != nil {
		t.Fatalf("Run again: %v", err)
	}
	again := outcomeFor(t, second, claim)
	if again.Issued || again.Reason != kasse.DunningSkipAlreadyDunned {
		t.Fatalf("second outcome = %+v, want already_dunned", again)
	}
	var notices int
	if err := testPool(t).QueryRow(ctx,
		"SELECT count(*) FROM dunning_notice WHERE claim_reference = $1", claim).Scan(&notices); err != nil {
		t.Fatalf("count notices: %v", err)
	}
	if notices != 1 {
		t.Fatalf("notices for the claim = %d, want 1", notices)
	}
}

func TestDunningRunOnlyWalksItemsDueByItsDate(t *testing.T) {
	ctx := context.Background()
	run, rec, year := newRunFixture(t)

	// An as-of date before the item's due date: the item is not yet walked.
	result, err := run.Run(ctx, year, time.Date(year-2, 6, 1, 0, 0, 0, 0, time.UTC), "clerk")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, o := range result.Outcomes {
		if o.ReceivableID == rec.ID {
			t.Fatalf("an item due after the as-of date was walked: %+v", o)
		}
	}
}

func TestDunningRunRefusesAChainItCannotWalk(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	run := kasse.NewDunningRunService(
		kasse.NewDunningRepo(pool), kasse.NewDunningRepo(pool), kasse.NewReceivableRepo(pool),
		kasse.NewDunningService(kasse.NewDunningRepo(pool), audit.NewStore(pool)), audit.NewStore(pool))

	// A year with no levels has no first level: the run refuses rather than
	// dunning a claim against nothing. 2999 lies outside the fixtures' 2000..2899.
	if _, err := run.Run(ctx, 2999, time.Date(2998, 1, 1, 0, 0, 0, 0, time.UTC), "clerk"); !errors.Is(err, kasse.ErrDunningChainHead) {
		t.Fatalf("err = %v, want ErrDunningChainHead", err)
	}
}
