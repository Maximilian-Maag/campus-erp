package kasse_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func TestReceivableNeedsAnExistingMandate(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	_, err := recv.Post(ctx, "MAN-absent-"+uniqueSuffix(t), 100, "EUR",
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Ohne Mandat", "", "clerk")
	if !errors.Is(err, kasse.ErrMandateNotFound) {
		t.Fatalf("unknown mandate = %v, want ErrMandateNotFound", err)
	}
}

func TestCollectionRunSelectsReceivables(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-OP-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	mkMandate := func(tag, iban string, seq kasse.MandateSequence) string {
		ref := "MAN-" + tag + "-" + uniqueSuffix(t)
		m, err := kasse.NewMandate(ref, "Debtor "+tag, iban, "COBADEFFXXX", sig, seq)
		if err != nil {
			t.Fatalf("NewMandate: %v", err)
		}
		if _, err := mandates.CreateMandate(ctx, m, "clerk"); err != nil {
			t.Fatalf("CreateMandate: %v", err)
		}
		return ref
	}
	manA := mkMandate("A", "DE89370400440532013000", kasse.SequenceRecurring)
	manB := mkMandate("B", "GB29NWBK60161331926819", kasse.SequenceRecurring)
	manFRST := mkMandate("F", "DE02120300000000202051", kasse.SequenceFirst)
	manInactive := mkMandate("I", "DE02500105170137075030", kasse.SequenceRecurring)
	if _, err := pool.Exec(ctx, `UPDATE direct_debit_mandate SET active = false WHERE mandate_reference = $1`, manInactive); err != nil {
		t.Fatalf("deactivate mandate: %v", err)
	}

	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	post := func(mand string, cents int64, due string, purpose string) int64 {
		d, err := time.Parse(time.DateOnly, due)
		if err != nil {
			t.Fatalf("due date: %v", err)
		}
		saved, err := recv.Post(ctx, mand, cents, "EUR", d, purpose, "test", "clerk")
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		return saved.ID
	}
	// Two items under one mandate, one under another; three that must not be
	// selected, each for a different reason.
	idA1 := post(manA, 3000, "2026-10-01", "Beitrag")
	idA2 := post(manA, 1250, "2026-10-15", "Miete")
	idB1 := post(manB, 500, "2026-10-10", "Gebuehr")
	post(manA, 999, "2026-11-30", "Zu spaet")         // due after due_by
	post(manFRST, 777, "2026-10-05", "Falsche SeqTp") // mandate sequence differs
	post(manInactive, 888, "2026-10-05", "Inaktiv")   // mandate inactive

	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	dueBy := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	ref := "RUN-" + uniqueSuffix(t)
	run, err := svc.CreateCollectionRunFromReceivables(ctx, ref, account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE", dueBy, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRunFromReceivables: %v", err)
	}
	// One collection per mandate, the mandate's items summed.
	if run.ItemCount != 2 || run.SumCents != 4750 {
		t.Fatalf("run = %+v, want 2 items / 4750", run)
	}
	file, err := svc.CollectionRunFile(ctx, ref)
	if err != nil {
		t.Fatalf("CollectionRunFile: %v", err)
	}
	for _, want := range []string{"<MndtId>" + manA + "</MndtId>", "<MndtId>" + manB + "</MndtId>"} {
		if !strings.Contains(file, want) {
			t.Errorf("file missing %q", want)
		}
	}
	for _, excluded := range []string{manFRST, manInactive} {
		if strings.Contains(file, excluded) {
			t.Errorf("file contains an excluded mandate %s", excluded)
		}
	}

	// The selected items are collected and point at the run.
	repo := kasse.NewReceivableRepo(pool)
	for _, id := range []int64{idA1, idA2, idB1} {
		got, err := repo.GetReceivable(ctx, id)
		if err != nil {
			t.Fatalf("GetReceivable(%d): %v", id, err)
		}
		if got.State != kasse.ReceivableCollected || got.CollectedRunID == nil || *got.CollectedRunID != run.ID {
			t.Fatalf("receivable %d = %+v, want collected by run %d", id, got, run.ID)
		}
	}

	// What was collected is no longer selectable, so a second run has nothing.
	if _, err := svc.CreateCollectionRunFromReceivables(ctx, "RUN2-"+uniqueSuffix(t), account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE", dueBy, "clerk"); !errors.Is(err, kasse.ErrNoCollectionItems) {
		t.Fatalf("second run = %v, want ErrNoCollectionItems", err)
	}

	// Idempotent on the reference: the same run, not a second one.
	same, err := svc.CreateCollectionRunFromReceivables(ctx, ref, account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE", dueBy, "clerk")
	if err != nil || same.ID != run.ID || same.FileHash != run.FileHash {
		t.Fatalf("re-run not idempotent: %+v, %v", same, err)
	}

	// The open list reads by due date and can be narrowed to the open ones.
	open, err := recv.Receivables(ctx, kasse.ReceivableOpen)
	if err != nil {
		t.Fatalf("Receivables: %v", err)
	}
	for i := 1; i < len(open); i++ {
		if open[i-1].DueDate.After(open[i].DueDate) {
			t.Fatalf("receivables not sorted by due date: %v before %v", open[i-1].DueDate, open[i].DueDate)
		}
	}
	for _, r := range open {
		if r.State != kasse.ReceivableOpen {
			t.Fatalf("open list contains a %s item", r.State)
		}
	}

	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestCollectionRunFromReceivablesInputChecks(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-Check-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	// A due_by after the collection date would collect an item not yet due.
	if _, err := svc.CreateCollectionRunFromReceivables(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE", date.Add(24*time.Hour), "clerk"); !errors.Is(err, kasse.ErrReceivableDueByAfterCollection) {
		t.Fatalf("late due_by = %v, want ErrReceivableDueByAfterCollection", err)
	}
	// A missing due_by is refused too.
	if _, err := svc.CreateCollectionRunFromReceivables(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE", time.Time{}, "clerk"); !errors.Is(err, kasse.ErrReceivableDueByRequired) {
		t.Fatalf("missing due_by = %v, want ErrReceivableDueByRequired", err)
	}
	// Nothing due is not an empty file.
	if _, err := svc.CreateCollectionRunFromReceivables(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), "clerk"); !errors.Is(err, kasse.ErrNoCollectionItems) {
		t.Fatalf("nothing due = %v, want ErrNoCollectionItems", err)
	}
}

func TestCollectionRunFromReceivablesRefusesForeignCurrency(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-FX-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	man := "MAN-" + uniqueSuffix(t)
	m, err := kasse.NewMandate(man, "Debtor", "DE89370400440532013000", "COBADEFFXXX", time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC), kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := kasse.NewMandateService(kasse.NewMandateRepo(pool), store).CreateMandate(ctx, m, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	if _, err := recv.Post(ctx, man, 1000, "USD", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Fremdwaehrung", "test", "clerk"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.CreateCollectionRunFromReceivables(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE", date, "clerk"); !errors.Is(err, kasse.ErrReceivableCurrencyMismatch) {
		t.Fatalf("foreign currency = %v, want ErrReceivableCurrencyMismatch", err)
	}
}

// TestCollectionRunRepoSRefusesStaleClaim drives the repository directly: a run
// that claims an item another run already took must fail and leave no run behind.
func TestCollectionRunRepoRefusesStaleClaim(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-Stale-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	man := "MAN-" + uniqueSuffix(t)
	m, err := kasse.NewMandate(man, "Debtor", "DE89370400440532013000", "COBADEFFXXX", time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC), kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := kasse.NewMandateService(kasse.NewMandateRepo(pool), store).CreateMandate(ctx, m, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	saved, err := recv.Post(ctx, man, 1000, "EUR", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "Beitrag", "test", "clerk")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	repo := kasse.NewCollectionRunRepo(pool)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	items := []kasse.CollectionRunItem{{MandateReference: man, EndToEndID: "E", AmountCents: 1000, Purpose: "Beitrag"}}
	ref1 := "RUN-" + uniqueSuffix(t)
	run := kasse.CollectionRun{Reference: ref1, AccountID: account.ID, CreditorID: "CID", CollectionDate: date,
		SequenceType: kasse.SequenceRecurring, LocalInstrument: "CORE", MessageID: ref1, ItemCount: 1,
		SumCents: 1000, FileHash: "h", CreatedBy: "clerk"}
	if _, err := repo.InsertCollectionRunFromReceivables(ctx, run, "<x/>", items, []int64{saved.ID}); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	ref2 := "RUN-" + uniqueSuffix(t)
	run2 := run
	run2.Reference, run2.MessageID = ref2, ref2
	if _, err := repo.InsertCollectionRunFromReceivables(ctx, run2, "<x/>", items, []int64{saved.ID}); !errors.Is(err, kasse.ErrReceivableNotOpen) {
		t.Fatalf("stale claim = %v, want ErrReceivableNotOpen", err)
	}
	// The refused run was not stored: the claim and the run are one transaction.
	if _, ok, err := repo.CollectionRunByReference(ctx, ref2); err != nil || ok {
		t.Fatalf("the refused run was stored: ok=%v err=%v", ok, err)
	}
}
