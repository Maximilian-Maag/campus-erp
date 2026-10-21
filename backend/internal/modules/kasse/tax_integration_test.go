package kasse_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

func newTaxService(t *testing.T) *kasse.TaxService {
	t.Helper()
	pool := testPool(t)
	return kasse.NewTaxService(kasse.NewTaxRepo(pool), audit.NewStore(pool))
}

func TestVATTypeRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc := newTaxService(t)

	code := "T" + uniqueSuffix(t)
	vat, err := kasse.NewVATType(code, "Teststeuersatz", 700, kasse.VATInput)
	if err != nil {
		t.Fatalf("NewVATType: %v", err)
	}
	saved, err := svc.CreateVATType(ctx, vat, "alice")
	if err != nil {
		t.Fatalf("CreateVATType: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("id not assigned")
	}

	reloaded, err := svc.VATType(ctx, code)
	if err != nil {
		t.Fatalf("VATType: %v", err)
	}
	if reloaded.RateBasisPoints != 700 || reloaded.Kind != kasse.VATInput {
		t.Fatalf("reloaded = %+v", reloaded)
	}
	// The reloaded type must be usable for arithmetic.
	if got, _ := reloaded.GrossFromNet(10000); got != 10700 {
		t.Fatalf("gross = %d, want 10700", got)
	}

	if got := countSubject(t, testPool(t), "vat-type/"+code); got != 1 {
		t.Fatalf("audit events = %d, want 1", got)
	}

	// A duplicate code is a conflict, not an opaque error.
	if _, err := svc.CreateVATType(ctx, vat, "alice"); !errors.Is(err, kasse.ErrVATCodeExists) {
		t.Fatalf("want ErrVATCodeExists, got %v", err)
	}
	if _, err := svc.VATType(ctx, "no-such-code"); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestTaxOfficeAndFoundationPersist(t *testing.T) {
	ctx := context.Background()
	svc := newTaxService(t)
	suffix := uniqueSuffix(t)

	office, err := kasse.NewTaxOffice(2026, suffix, "Finanzamt Musterstadt",
		"Steuerweg 1", "12345", "Musterstadt", "Land Muster", "DE02120300000000202051", "BYLADEM1001")
	if err != nil {
		t.Fatalf("NewTaxOffice: %v", err)
	}
	savedOffice, err := svc.CreateTaxOffice(ctx, office, "alice")
	if err != nil {
		t.Fatalf("CreateTaxOffice: %v", err)
	}
	if savedOffice.IBAN != "DE02120300000000202051" {
		t.Fatalf("iban = %q", savedOffice.IBAN)
	}
	if _, err := svc.CreateTaxOffice(ctx, office, "alice"); !errors.Is(err, kasse.ErrTaxOfficeExists) {
		t.Fatalf("want ErrTaxOfficeExists, got %v", err)
	}

	foundation, err := kasse.NewFoundation(2026, suffix, "Deutschlandstipendium", true)
	if err != nil {
		t.Fatalf("NewFoundation: %v", err)
	}
	savedFoundation, err := svc.CreateFoundation(ctx, foundation, "alice")
	if err != nil {
		t.Fatalf("CreateFoundation: %v", err)
	}
	if !savedFoundation.CarryOver {
		t.Fatal("carry_over not persisted")
	}
	if _, err := svc.CreateFoundation(ctx, foundation, "alice"); !errors.Is(err, kasse.ErrFoundationExists) {
		t.Fatalf("want ErrFoundationExists, got %v", err)
	}
}
