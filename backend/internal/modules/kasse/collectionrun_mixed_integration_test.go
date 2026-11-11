package kasse_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestCollectionRunMixesSequenceTypes covers a run that declares no sequence type: it collects each
// mandate under the mandate's own type, so one file carries one payment instruction per type instead
// of forcing an operator to file two runs and hand out two files for one collection date.
func TestCollectionRunMixesSequenceTypes(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-Mix-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
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
	first := mkMandate("first", "DE89370400440532013000", kasse.SequenceFirst)
	recurring := mkMandate("recurring", "GB29NWBK60161331926819", kasse.SequenceRecurring)

	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ref := "RUN-mix-" + uniqueSuffix(t)
	items := []kasse.CollectionItem{
		{MandateReference: first, EndToEndID: "E2E-F", AmountCents: 4250, Purpose: "Erstbeitrag"},
		{MandateReference: recurring, EndToEndID: "E2E-R", AmountCents: 1250, Purpose: "Folgebeitrag"},
	}

	// An empty sequence type means "each mandate's own", not "no type given".
	run, err := svc.CreateCollectionRun(ctx, ref, account.ID, "DE98ZZZ09999999999", date, "", "CORE", items, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRun: %v", err)
	}
	if !run.MixedSequence() || run.SequenceType != "" {
		t.Fatalf("run must carry no single type: %+v", run)
	}
	if run.ItemCount != 2 || run.SumCents != 5500 {
		t.Fatalf("run totals = %d/%d, want 2/5500", run.ItemCount, run.SumCents)
	}

	file, err := svc.CollectionRunFile(ctx, ref)
	if err != nil {
		t.Fatalf("CollectionRunFile: %v", err)
	}
	if n := strings.Count(file, "<PmtInf>"); n != 2 {
		t.Fatalf("PmtInf blocks = %d, want 2\n%s", n, file)
	}
	// The first collection is read before the recurring one it precedes, and each instruction names
	// itself so a bank can address one of them.
	firstAt := strings.Index(file, "<PmtInfId>"+ref+"-FRST</PmtInfId>")
	recurringAt := strings.Index(file, "<PmtInfId>"+ref+"-RCUR</PmtInfId>")
	if firstAt < 0 || recurringAt < 0 || firstAt > recurringAt {
		t.Fatalf("instruction ids/order wrong (FRST at %d, RCUR at %d)\n%s", firstAt, recurringAt, file)
	}
	for _, want := range []string{
		"<SeqTp>FRST</SeqTp>", "<SeqTp>RCUR</SeqTp>",
		"<MndtId>" + first + "</MndtId>", "<MndtId>" + recurring + "</MndtId>",
		// The header totals both instructions.
		"<NbOfTxs>2</NbOfTxs>", "<CtrlSum>55.00</CtrlSum>",
	} {
		if !strings.Contains(file, want) {
			t.Errorf("file missing %q", want)
		}
	}

	// The stored run reads back as mixed rather than as one of the two types.
	loaded, err := svc.CollectionRun(ctx, ref)
	if err != nil {
		t.Fatalf("CollectionRun: %v", err)
	}
	if !loaded.MixedSequence() {
		t.Fatalf("stored run = %q, want no single type", loaded.SequenceType)
	}

	// A run that declares one type is unchanged: one instruction, no derived identifier, so files
	// already handed to a bank keep their shape.
	oneRef := "RUN-one-" + uniqueSuffix(t)
	one, err := svc.CreateCollectionRun(ctx, oneRef, account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE",
		[]kasse.CollectionItem{{MandateReference: recurring, EndToEndID: "E2E-R", AmountCents: 1250, Purpose: "Folgebeitrag"}}, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRun(one type): %v", err)
	}
	if one.SequenceType != kasse.SequenceRecurring || one.MixedSequence() {
		t.Fatalf("single-type run = %q, want RCUR", one.SequenceType)
	}
	oneFile, err := svc.CollectionRunFile(ctx, oneRef)
	if err != nil {
		t.Fatalf("CollectionRunFile(one type): %v", err)
	}
	if n := strings.Count(oneFile, "<PmtInf>"); n != 1 {
		t.Fatalf("PmtInf blocks = %d, want 1\n%s", n, oneFile)
	}
	if !strings.Contains(oneFile, "<PmtInfId>"+oneRef+"</PmtInfId>") {
		t.Errorf("a single instruction must keep the run's reference as its id:\n%s", oneFile)
	}
}

// TestCollectionRunSelectsReceivablesOfEveryType covers the selecting run without a sequence type:
// it gathers the open items of every type rather than only those of one.
func TestCollectionRunSelectsReceivablesOfEveryType(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Uni-Sel-"+uniqueSuffix(t), "DE02120300000000202051", "BYLADEM1001", "EUR")
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
	first := mkMandate("sel-first", "DE89370400440532013000", kasse.SequenceFirst)
	recurring := mkMandate("sel-recurring", "GB29NWBK60161331926819", kasse.SequenceRecurring)

	due, err := time.Parse(time.DateOnly, "2026-10-01")
	if err != nil {
		t.Fatalf("parse due date: %v", err)
	}
	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	for _, m := range []struct {
		ref     string
		cents   int64
		purpose string
	}{{first, 4250, "Erstbeitrag"}, {recurring, 1250, "Folgebeitrag"}} {
		if _, err := recv.Post(ctx, m.ref, m.cents, "EUR", due, m.purpose, "", "clerk"); err != nil {
			t.Fatalf("Post(%s): %v", m.ref, err)
		}
	}

	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store)
	ref := "RUN-sel-" + uniqueSuffix(t)
	run, err := svc.CreateCollectionRunFromReceivables(ctx, ref, account.ID, "DE98ZZZ09999999999",
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "", "CORE", due, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRunFromReceivables: %v", err)
	}
	if run.ItemCount != 2 || run.SumCents != 5500 {
		t.Fatalf("selected run = %d items, %d cents; want 2/5500", run.ItemCount, run.SumCents)
	}
	file, err := svc.CollectionRunFile(ctx, ref)
	if err != nil {
		t.Fatalf("CollectionRunFile: %v", err)
	}
	if n := strings.Count(file, "<PmtInf>"); n != 2 {
		t.Fatalf("PmtInf blocks = %d, want 2\n%s", n, file)
	}
}
