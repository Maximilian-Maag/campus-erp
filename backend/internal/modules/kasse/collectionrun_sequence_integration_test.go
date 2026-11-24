package kasse_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// seqFixture is one mandate, one cash account and the services a collection-run test needs.
type seqFixture struct {
	pool      *pgxpool.Pool
	runSvc    *kasse.CollectionRunService
	mandates  *kasse.MandateService
	account   int64
	reference string
}

func newSeqFixture(t *testing.T, seq kasse.MandateSequence) seqFixture {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	account, err := cash.CreateCashAccount(ctx, "Universitaet Musterstadt",
		"DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	ref := "MAN-" + uniqueSuffix(t)
	m, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "COBADEFFXXX",
		time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC), seq)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := mandates.CreateMandate(ctx, m, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	return seqFixture{
		pool:      pool,
		runSvc:    kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool), kasse.NewMandateRepo(pool), kasse.NewReceivableRepo(pool), store),
		mandates:  mandates,
		account:   account.ID,
		reference: ref,
	}
}

// collect produces one run for the fixture's mandate under the given sequence type and returns the
// stored file.
func (f seqFixture) collect(t *testing.T, seq kasse.MandateSequence) string {
	t.Helper()
	runRef := "RUN-" + uniqueSuffix(t)
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	items := []kasse.CollectionItem{
		{MandateReference: f.reference, EndToEndID: "E2E-" + uniqueSuffix(t), AmountCents: 425, Purpose: "Semesterbeitrag"},
	}
	if _, err := f.runSvc.CreateCollectionRun(context.Background(), runRef, f.account, "DE98ZZZ09999999999",
		date, seq, "CORE", items, "clerk"); err != nil {
		t.Fatalf("CreateCollectionRun(%s): %v", seq, err)
	}
	file, err := f.runSvc.CollectionRunFile(context.Background(), runRef)
	if err != nil {
		t.Fatalf("CollectionRunFile: %v", err)
	}
	return file
}

// A mandate registered FRST may be collected as FRST once. The run advances it to RCUR, so the
// second run is a recurring collection — the Direct Debit Rulebook forbids a second FRST under the
// same MdtId, and before this the row never moved.
func TestACollectionRunAdvancesAFrstMandateToRcur(t *testing.T) {
	f := newSeqFixture(t, kasse.SequenceFirst)

	first := f.collect(t, kasse.SequenceFirst)
	if !strings.Contains(first, "<SeqTp>FRST</SeqTp>") {
		t.Fatalf("first file is not a FRST collection: %s", first)
	}

	after, err := f.mandates.Mandate(context.Background(), f.reference)
	if err != nil {
		t.Fatalf("Mandate: %v", err)
	}
	if after.SequenceType != kasse.SequenceRecurring {
		t.Fatalf("mandate sequence type after the first run = %q, want RCUR", after.SequenceType)
	}

	second := f.collect(t, kasse.SequenceRecurring)
	if !strings.Contains(second, "<SeqTp>RCUR</SeqTp>") {
		t.Fatalf("second file is not a RCUR collection: %s", second)
	}
	if strings.Contains(second, "<SeqTp>FRST</SeqTp>") {
		t.Fatalf("second file still collects FRST: %s", second)
	}

	if err := audit.NewStore(f.pool).Verify(context.Background()); err != nil {
		t.Fatalf("audit chain: %v", err)
	}
}

// A recurring mandate stays recurring: only a first collection moves, so a second run of the same
// mandate leaves the type alone.
func TestACollectionRunLeavesARecurringMandateAlone(t *testing.T) {
	f := newSeqFixture(t, kasse.SequenceRecurring)
	f.collect(t, kasse.SequenceRecurring)
	after, err := f.mandates.Mandate(context.Background(), f.reference)
	if err != nil {
		t.Fatalf("Mandate: %v", err)
	}
	if after.SequenceType != kasse.SequenceRecurring {
		t.Fatalf("sequence type = %q, want RCUR", after.SequenceType)
	}
}

// Marking the mandate final is how an operator asks for the last collection; the run then emits
// FNAL, which was unreachable before.
func TestMarkFinalCollectsTheLastTimeAsFnal(t *testing.T) {
	ctx := context.Background()
	f := newSeqFixture(t, kasse.SequenceRecurring)

	final, err := f.mandates.MarkFinal(ctx, f.reference, "clerk")
	if err != nil {
		t.Fatalf("MarkFinal: %v", err)
	}
	if final.SequenceType != kasse.SequenceFinal {
		t.Fatalf("sequence type = %q, want FNAL", final.SequenceType)
	}

	file := f.collect(t, kasse.SequenceFinal)
	if !strings.Contains(file, "<SeqTp>FNAL</SeqTp>") {
		t.Fatalf("final file is not a FNAL collection: %s", file)
	}

	// Asking twice is refused: the last collection has already been announced.
	if _, err := f.mandates.MarkFinal(ctx, f.reference, "clerk"); !errors.Is(err, kasse.ErrMandateAlreadyFinal) {
		t.Fatalf("second MarkFinal = %v, want ErrMandateAlreadyFinal", err)
	}

	if err := audit.NewStore(f.pool).Verify(ctx); err != nil {
		t.Fatalf("audit chain: %v", err)
	}
}

// A mandate that has never been collected cannot be marked final: FNAL announces the end of an
// authorisation that has not been used yet.
func TestMarkFinalRefusesAFrstMandate(t *testing.T) {
	f := newSeqFixture(t, kasse.SequenceFirst)
	if _, err := f.mandates.MarkFinal(context.Background(), f.reference, "clerk"); !errors.Is(err, kasse.ErrMandateNotYetCollected) {
		t.Fatalf("MarkFinal on a FRST mandate = %v, want ErrMandateNotYetCollected", err)
	}
}
