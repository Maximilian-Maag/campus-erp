package kasse_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func TestCollectionRunProducesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Universitaet Musterstadt", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}

	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	m1 := "MAN-" + uniqueSuffix(t)
	m2 := m1 + "b"
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	for _, m := range []struct{ ref, name, iban string }{
		{m1, "Max Mustermann", "DE89370400440532013000"},
		{m2, "Erika Musterfrau", "GB29NWBK60161331926819"},
	} {
		mand, err := kasse.NewMandate(m.ref, m.name, m.iban, "COBADEFFXXX", sig, kasse.SequenceRecurring)
		if err != nil {
			t.Fatalf("NewMandate(%s): %v", m.ref, err)
		}
		if _, err := mandates.CreateMandate(ctx, mand, "clerk"); err != nil {
			t.Fatalf("CreateMandate(%s): %v", m.ref, err)
		}
	}

	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), store)
	runRef := "RUN-" + uniqueSuffix(t)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	items := []kasse.CollectionItem{
		{MandateReference: m1, EndToEndID: "E2E-1", AmountCents: 4250, Purpose: "Semesterbeitrag"},
		{MandateReference: m2, EndToEndID: "E2E-2", AmountCents: 1250, Purpose: "Bibliotheksgebuehr"},
	}

	run, err := svc.CreateCollectionRun(ctx, runRef, account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE", items, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRun: %v", err)
	}
	if run.ItemCount != 2 || run.SumCents != 5500 {
		t.Fatalf("run = %+v", run)
	}

	file, err := svc.CollectionRunFile(ctx, runRef)
	if err != nil {
		t.Fatalf("CollectionRunFile: %v", err)
	}
	for _, want := range []string{
		"urn:iso:std:iso:20022:tech:xsd:pain.008.001.08",
		"<PmtMtd>DD</PmtMtd>",
		"<SeqTp>RCUR</SeqTp>",
		"<MndtId>" + m1 + "</MndtId>",
		"<MndtId>" + m2 + "</MndtId>",
	} {
		if !strings.Contains(file, want) {
			t.Errorf("file missing %q", want)
		}
	}
	sum := sha256.Sum256([]byte(file))
	if run.FileHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("file hash mismatch: %s", run.FileHash)
	}

	// Idempotent on the reference: same run, no second file.
	again, err := svc.CreateCollectionRun(ctx, runRef, account.ID, "DE98ZZZ09999999999", date, kasse.SequenceRecurring, "CORE", items, "clerk")
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if again.ID != run.ID || again.FileHash != run.FileHash {
		t.Fatalf("re-run not idempotent: %+v vs %+v", again, run)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestCollectionRunFailsClosed(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Universitaet Musterstadt", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	svc := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), store)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)

	// Unknown mandate.
	_, err = svc.CreateCollectionRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE",
		[]kasse.CollectionItem{{MandateReference: "MAN-absent-" + uniqueSuffix(t), EndToEndID: "E", AmountCents: 100, Purpose: "x"}}, "clerk")
	if !errors.Is(err, kasse.ErrMandateNotFound) {
		t.Fatalf("unknown mandate = %v, want ErrMandateNotFound", err)
	}

	// Sequence-type mismatch (FRST mandate, RCUR run).
	ref := "MAN-" + uniqueSuffix(t)
	mand, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "COBADEFFXXX", sig, kasse.SequenceFirst)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := mandates.CreateMandate(ctx, mand, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	_, err = svc.CreateCollectionRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, "CID", date, kasse.SequenceRecurring, "CORE",
		[]kasse.CollectionItem{{MandateReference: ref, EndToEndID: "E", AmountCents: 100, Purpose: "x"}}, "clerk")
	if !errors.Is(err, kasse.ErrMandateSequenceMismatch) {
		t.Fatalf("mismatch = %v, want ErrMandateSequenceMismatch", err)
	}

	// The DB-level per-run mandate uniqueness is a second guard (the service
	// dedupes first); drive the repo directly to exercise it.
	repo := kasse.NewCollectionRunRepo(pool)
	runRef := "RUN-dup-" + uniqueSuffix(t)
	_, err = repo.InsertCollectionRun(ctx, kasse.CollectionRun{
		Reference: runRef, AccountID: account.ID, CreditorID: "CID", CollectionDate: date,
		SequenceType: kasse.SequenceFirst, LocalInstrument: "CORE", MessageID: runRef,
		ItemCount: 2, SumCents: 200, FileHash: "x", CreatedBy: "clerk",
	}, "<xml/>", []kasse.CollectionRunItem{
		{MandateReference: ref, EndToEndID: "E1", AmountCents: 100, Purpose: "x"},
		{MandateReference: ref, EndToEndID: "E2", AmountCents: 100, Purpose: "y"},
	})
	if !errors.Is(err, kasse.ErrDuplicateMandate) {
		t.Fatalf("duplicate mandate = %v, want ErrDuplicateMandate", err)
	}
}
