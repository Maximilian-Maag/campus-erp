package klr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// TestInternalPricingChargesOut: a service is priced and charged out. The provider is relieved by
// exactly what the receiver is charged, so the charge moves cost between the two without changing
// the total the centres carry.
func TestInternalPricingChargesOut(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	provider, err := svc.CreateCostCenter(ctx, "P-"+mark, "Rechenzentrum", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	receiver, err := svc.CreateCostCenter(ctx, "R-"+mark, "Bibliothek", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	ct, err := svc.PostCostType(ctx, "4711-"+mark, "Personalkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}

	// The provider carries a cost of its own, which the charge will relieve.
	if _, err := svc.Post(ctx, provider.ID, nil, nil, klr.Cost, 100000, klrDay(2026, 4, 10), "RE-"+mark, "alice"); err != nil {
		t.Fatalf("Post: %v", err)
	}
	before, err := svc.Report(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}

	rate, err := svc.DefineServiceRate(ctx, ct.ID, 2500, klrDay(2026, 1, 1), nil, "alice")
	if err != nil {
		t.Fatalf("DefineServiceRate: %v", err)
	}
	if rate.RateCents != 2500 {
		t.Fatalf("rate = %+v", rate)
	}
	rates, err := svc.ServiceRates(ctx)
	if err != nil || len(rates) == 0 {
		t.Fatalf("ServiceRates = %+v, %v", rates, err)
	}

	// Three units at 25,00 is 75,00.
	charge, err := svc.ChargeService(ctx, provider.ID, receiver.ID, ct.ID, 3, klrDay(2026, 4, 30), "alice")
	if err != nil {
		t.Fatalf("ChargeService: %v", err)
	}
	if charge.AmountCents != 7500 || charge.Quantity != 3 {
		t.Fatalf("charge = %+v", charge)
	}

	// The provider is relieved by exactly that: its balance moves up by the charge.
	after, err := svc.Report(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if after.BalanceCents()-before.BalanceCents() != 7500 {
		t.Fatalf("provider balance moved by %d, want 7500", after.BalanceCents()-before.BalanceCents())
	}
	// And the receiver carries it.
	recv, err := svc.Report(ctx, receiver.ID)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if recv.CostsCents != 7500 || recv.BalanceCents() != -7500 {
		t.Fatalf("receiver = %+v", recv)
	}

	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestInternalPricingRefusals: the store decides what a charge can be, not just the caller.
func TestInternalPricingRefusals(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	provider, err := svc.CreateCostCenter(ctx, "P-"+mark, "Rechenzentrum", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	receiver, err := svc.CreateCostCenter(ctx, "R-"+mark, "Bibliothek", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	ct, err := svc.PostCostType(ctx, "5000-"+mark, "Sachkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}

	// A rate for a cost type that does not exist is refused.
	if _, err := svc.DefineServiceRate(ctx, 9223372036854775807, 2500, klrDay(2026, 1, 1), nil, "alice"); !errors.Is(err, klr.ErrCostTypeNotFound) {
		t.Fatalf("unknown cost type = %v, want ErrCostTypeNotFound", err)
	}

	// With no rate in force there is nothing to charge.
	if _, err := svc.ChargeService(ctx, provider.ID, receiver.ID, ct.ID, 1, klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrNoRateInForce) {
		t.Fatalf("no rate = %v, want ErrNoRateInForce", err)
	}

	// A rate that has already closed does not price a later charge.
	if _, err := svc.DefineServiceRate(ctx, ct.ID, 2500, klrDay(2026, 1, 1), ptrTime(klrDay(2026, 3, 1)), "alice"); err != nil {
		t.Fatalf("DefineServiceRate: %v", err)
	}
	if _, err := svc.ChargeService(ctx, provider.ID, receiver.ID, ct.ID, 1, klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrNoRateInForce) {
		t.Fatalf("expired rate = %v, want ErrNoRateInForce", err)
	}

	// A rate that starts later takes over from the closed one without deleting it: the day in
	// between the two is priced by the later rate.
	if _, err := svc.DefineServiceRate(ctx, ct.ID, 3000, klrDay(2026, 4, 1), nil, "alice"); err != nil {
		t.Fatalf("DefineServiceRate: %v", err)
	}
	charge, err := svc.ChargeService(ctx, provider.ID, receiver.ID, ct.ID, 2, klrDay(2026, 4, 30), "alice")
	if err != nil {
		t.Fatalf("ChargeService: %v", err)
	}
	if charge.AmountCents != 6000 {
		t.Fatalf("amount = %d, want 6000 (2 units at the newer rate)", charge.AmountCents)
	}

	// A centre cannot charge itself.
	if _, err := svc.ChargeService(ctx, provider.ID, provider.ID, ct.ID, 1, klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrServiceChargeSelf) {
		t.Fatalf("self charge = %v, want ErrServiceChargeSelf", err)
	}
	// And a charge for nothing is not a charge.
	if _, err := svc.ChargeService(ctx, provider.ID, receiver.ID, ct.ID, 0, klrDay(2026, 4, 30), "alice"); !errors.Is(err, klr.ErrQuantityNotPositive) {
		t.Fatalf("no quantity = %v, want ErrQuantityNotPositive", err)
	}
}
