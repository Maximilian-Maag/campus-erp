package kasse_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func TestMandatePersistsAndIsAudited(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewMandateService(kasse.NewMandateRepo(pool), audit.NewStore(pool))
	ref := "MAN-" + uniqueSuffix(t)
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)

	m, err := kasse.NewMandate(ref, "Max Mustermann", "DE89370400440532013000", "COBADEFFXXX", sig, kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	saved, err := svc.CreateMandate(ctx, m, "alice")
	if err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}
	if saved.ID == 0 || !saved.Active {
		t.Fatalf("saved = %+v", saved)
	}

	got, err := svc.Mandate(ctx, ref)
	if err != nil {
		t.Fatalf("Mandate: %v", err)
	}
	if got.DebtorName != "Max Mustermann" || got.SequenceType != kasse.SequenceRecurring ||
		got.DebtorBIC != "COBADEFFXXX" || !got.SignatureDate.Equal(sig) {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// A duplicate reference is a conflict.
	if _, err := svc.CreateMandate(ctx, m, "alice"); !errors.Is(err, kasse.ErrMandateReferenceExists) {
		t.Fatalf("duplicate = %v, want ErrMandateReferenceExists", err)
	}
	// A missing reference is not found.
	if _, err := svc.Mandate(ctx, ref+"-absent"); !errors.Is(err, kasse.ErrMandateNotFound) {
		t.Fatalf("missing = %v, want ErrMandateNotFound", err)
	}
	// A blank actor is rejected before any write.
	if _, err := svc.CreateMandate(ctx, kasse.Mandate{Reference: ref + "-x"}, "  "); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("actor = %v, want ErrActorRequired", err)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
