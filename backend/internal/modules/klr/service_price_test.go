package klr

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// newSvc wires the service to a repository with an audit sink that accepts every event.
func newSvc(repo Repository) *Service { return NewService(repo, okAudit{}) }

func TestNewServiceRateValidation(t *testing.T) {
	from := day(2026, 1, 1)
	until := day(2026, 12, 31)

	rate, err := NewServiceRate(4, 2500, from, &until)
	if err != nil {
		t.Fatalf("valid rate rejected: %v", err)
	}
	if rate.CostTypeID != 4 || rate.RateCents != 2500 || !rate.ValidFrom.Equal(from) {
		t.Fatalf("rate = %+v", rate)
	}
	if rate.ValidTo == nil || !rate.ValidTo.Equal(until) {
		t.Fatalf("valid_to = %v", rate.ValidTo)
	}

	cases := []struct {
		name       string
		costTypeID int64
		rateCents  int64
		from, to   time.Time
		want       error
	}{
		{"no cost type", 0, 2500, from, until, ErrCostTypeNotFound},
		{"zero rate", 4, 0, from, until, ErrRateNotPositive},
		{"negative rate", 4, -1, from, until, ErrRateNotPositive},
		{"period backwards", 4, 2500, until, from, ErrAllocationPeriod},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			to := c.to
			if _, err := NewServiceRate(c.costTypeID, c.rateCents, c.from, &to); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	// A rate with no end date is fine.
	if _, err := NewServiceRate(4, 1, from, nil); err != nil {
		t.Fatalf("open-ended rate: %v", err)
	}
}

func TestServiceRateActiveAndString(t *testing.T) {
	from := day(2026, 1, 1)
	until := day(2026, 3, 31)

	open, _ := NewServiceRate(4, 2500, from, nil)
	if open.Active(day(2025, 12, 31)) || !open.Active(from) || !open.Active(day(2030, 1, 1)) {
		t.Fatal("an open-ended rate applies from its valid_from onwards")
	}
	bounded, _ := NewServiceRate(4, 2500, from, &until)
	if !bounded.Active(from) || !bounded.Active(until) || bounded.Active(day(2026, 4, 1)) {
		t.Fatal("the whole validity range is inclusive and nothing after it")
	}

	bounded.ID = 7
	if got, want := bounded.String(), "service rate 7: cost type 4 at 2500 cents per unit"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestNewServiceChargeValidation(t *testing.T) {
	on := day(2026, 4, 30)

	charge, err := NewServiceCharge(1, 2, 4, 3, 2500, on)
	if err != nil {
		t.Fatalf("valid charge rejected: %v", err)
	}
	// The amount is the quantity priced at the rate, computed once and then stored.
	if charge.AmountCents != 7500 || charge.Quantity != 3 || charge.ProviderCenterID != 1 || charge.ReceiverCenterID != 2 {
		t.Fatalf("charge = %+v", charge)
	}

	cases := []struct {
		name                        string
		provider, receiver, ct, qty int64
		rateCents                   int64
		want                        error
	}{
		{"no provider", 0, 2, 4, 3, 2500, ErrCostCenterNotFound},
		{"no receiver", 1, 0, 4, 3, 2500, ErrCostCenterNotFound},
		{"charges itself", 1, 1, 4, 3, 2500, ErrServiceChargeSelf},
		{"no cost type", 1, 2, 0, 3, 2500, ErrCostTypeNotFound},
		{"no quantity", 1, 2, 4, 0, 2500, ErrQuantityNotPositive},
		{"negative quantity", 1, 2, 4, -1, 2500, ErrQuantityNotPositive},
		{"no rate", 1, 2, 4, 3, 0, ErrRateNotPositive},
		{"overflows", 1, 2, 4, 3, math.MaxInt64, ErrChargeTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewServiceCharge(c.provider, c.receiver, c.ct, c.qty, c.rateCents, on); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	// Exactly at the limit is not over it.
	if _, err := NewServiceCharge(1, 2, 4, 2, math.MaxInt64/2, on); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
}

func TestDefineServiceRate(t *testing.T) {
	ctx := context.Background()
	from := day(2026, 1, 1)
	repo := &fakeRepo{costType: CostType{ID: 4, Code: "4711"}}
	svc := newSvc(repo)

	saved, err := svc.DefineServiceRate(ctx, 4, 2500, from, nil, "alice")
	if err != nil {
		t.Fatalf("DefineServiceRate: %v", err)
	}
	if saved.ID != 1 || saved.RateCents != 2500 || saved.CreatedBy != "alice" {
		t.Fatalf("saved = %+v", saved)
	}

	if _, err := svc.DefineServiceRate(ctx, 4, 2500, from, nil, " "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v", err)
	}
	if _, err := svc.DefineServiceRate(ctx, 0, 2500, from, nil, "alice"); !errors.Is(err, ErrCostTypeNotFound) {
		t.Fatalf("no cost type = %v", err)
	}
	if _, err := svc.DefineServiceRate(ctx, 4, 0, from, nil, "alice"); !errors.Is(err, ErrRateNotPositive) {
		t.Fatalf("no rate = %v", err)
	}

	boom := errors.New("boom")
	if _, err := newSvc(&fakeRepo{err: boom}).DefineServiceRate(ctx, 4, 2500, from, nil, "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v", err)
	}
	if _, err := newSvc(&fakeRepo{costType: CostType{ID: 4}, err: nil}).DefineServiceRate(ctx, 4, 2500, from, nil, "alice"); err != nil {
		t.Fatalf("cost type lookup: %v", err)
	}
	// An audit that refuses the event stops the call.
	bad := newSvc(&fakeRepo{costType: CostType{ID: 4}})
	bad.audit = failingAudit{err: boom}
	if _, err := bad.DefineServiceRate(ctx, 4, 2500, from, nil, "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v", err)
	}
}

func TestServiceRatesList(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{rates: []ServiceRate{{ID: 1, CostTypeID: 4, RateCents: 2500}}}
	svc := newSvc(repo)

	rates, err := svc.ServiceRates(ctx)
	if err != nil || len(rates) != 1 || rates[0].RateCents != 2500 {
		t.Fatalf("ServiceRates = %+v, %v", rates, err)
	}
}

func TestChargeService(t *testing.T) {
	ctx := context.Background()
	on := day(2026, 4, 30)
	repo := &fakeRepo{
		center: CostCenter{ID: 1, Number: "K-10"},
		rate:   ServiceRate{ID: 2, CostTypeID: 4, RateCents: 2500, ValidFrom: day(2026, 1, 1)},
	}
	svc := newSvc(repo)

	charge, err := svc.ChargeService(ctx, 1, 2, 4, 3, on, "alice")
	if err != nil {
		t.Fatalf("ChargeService: %v", err)
	}
	if charge.ID != 1 || charge.AmountCents != 7500 || charge.ChargedBy != "alice" {
		t.Fatalf("charge = %+v", charge)
	}
	// The receiver is charged and the provider relieved, by exactly the same amount.
	if len(repo.chargeRecords) != 2 {
		t.Fatalf("records = %d, want 2 (%+v)", len(repo.chargeRecords), repo.chargeRecords)
	}
	posted, relief := repo.chargeRecords[0], repo.chargeRecords[1]
	if posted.CostCenterID != 2 || posted.Kind != Cost || posted.AmountCents != 7500 {
		t.Fatalf("receiver record = %+v", posted)
	}
	if relief.CostCenterID != 1 || relief.Kind != Revenue || relief.AmountCents != 7500 {
		t.Fatalf("relief record = %+v", relief)
	}
	if !posted.BusinessDate.Equal(on) || posted.BookedBy != "alice" {
		t.Fatalf("receiver record = %+v", posted)
	}

	if _, err := svc.ChargeService(ctx, 1, 2, 4, 3, on, " "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v", err)
	}
	// Both ends must exist, and the provider is looked up first.
	if _, err := newSvc(&fakeRepo{err: ErrCostCenterNotFound}).ChargeService(ctx, 1, 2, 4, 3, on, "alice"); !errors.Is(err, ErrCostCenterNotFound) {
		t.Fatalf("unknown centre = %v", err)
	}
	// No rate in force means nothing to charge.
	noRate := &fakeRepo{center: CostCenter{ID: 1}, err: ErrNoRateInForce}
	if _, err := newSvc(noRate).ChargeService(ctx, 1, 2, 4, 3, on, "alice"); !errors.Is(err, ErrNoRateInForce) {
		t.Fatalf("no rate = %v", err)
	}
	// The quantity is checked after the rate is found, so a bad quantity still reports as one.
	zero := &fakeRepo{center: CostCenter{ID: 1}, rate: ServiceRate{RateCents: 2500}}
	if _, err := newSvc(zero).ChargeService(ctx, 1, 2, 4, 0, on, "alice"); !errors.Is(err, ErrQuantityNotPositive) {
		t.Fatalf("no quantity = %v", err)
	}
	// A centre charging itself is refused even though both ends exist.
	if _, err := newSvc(zero).ChargeService(ctx, 1, 1, 4, 3, on, "alice"); !errors.Is(err, ErrServiceChargeSelf) {
		t.Fatalf("self charge = %v", err)
	}

	boom := errors.New("boom")
	if _, err := newSvc(&fakeRepo{center: CostCenter{ID: 1}, rate: ServiceRate{RateCents: 2500}, err: boom}).ChargeService(ctx, 1, 2, 4, 3, on, "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v", err)
	}
	bad := newSvc(&fakeRepo{center: CostCenter{ID: 1}, rate: ServiceRate{RateCents: 2500}})
	bad.audit = failingAudit{err: boom}
	if _, err := bad.ChargeService(ctx, 1, 2, 4, 3, on, "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v", err)
	}
}
