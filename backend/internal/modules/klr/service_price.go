package klr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// Service-rate errors.
var (
	// ErrRateNotPositive is returned when a rate is zero or negative.
	ErrRateNotPositive = errors.New("klr: the rate must be positive")
	// ErrQuantityNotPositive is returned when a service is charged out for no units.
	ErrQuantityNotPositive = errors.New("klr: the quantity must be positive")
	// ErrServiceChargeSelf is returned when a centre charges itself.
	ErrServiceChargeSelf = errors.New("klr: a cost centre cannot charge itself for its own service")
	// ErrNoRateInForce is returned when no rate covers the day a service is charged out on.
	ErrNoRateInForce = errors.New("klr: no service rate is in force for that cost type on that date")
	// ErrChargeTooLarge is returned when the quantity and the rate multiply past what the books
	// can hold.
	ErrChargeTooLarge = errors.New("klr: the charge is too large")
)

// ServiceRate is the price of one unit of a cost type's service, over a period.
type ServiceRate struct {
	ID         int64
	CostTypeID int64
	RateCents  int64
	ValidFrom  time.Time
	ValidTo    *time.Time
	CreatedBy  string
}

// NewServiceRate validates and builds a rate. The period rule is the keys' rule, so it is reported
// with the same error rather than a second one.
func NewServiceRate(costTypeID, rateCents int64, validFrom time.Time, validTo *time.Time) (ServiceRate, error) {
	if costTypeID <= 0 {
		return ServiceRate{}, ErrCostTypeNotFound
	}
	if rateCents <= 0 {
		return ServiceRate{}, ErrRateNotPositive
	}
	if validTo != nil && validTo.Before(validFrom) {
		return ServiceRate{}, ErrAllocationPeriod
	}
	return ServiceRate{CostTypeID: costTypeID, RateCents: rateCents, ValidFrom: validFrom, ValidTo: validTo}, nil
}

// Active reports whether the rate applies at a point in time. A rate with no end date is
// open-ended; the whole validity range is inclusive.
func (r ServiceRate) Active(at time.Time) bool {
	if at.Before(r.ValidFrom) {
		return false
	}
	return r.ValidTo == nil || !at.After(*r.ValidTo)
}

// String renders a rate for messages and logs.
func (r ServiceRate) String() string {
	return fmt.Sprintf("service rate %d: cost type %d at %d cents per unit", r.ID, r.CostTypeID, r.RateCents)
}

// ServiceCharge is a rendered service: one centre charged another for a quantity of a cost type's
// service, at the rate in force on the day.
type ServiceCharge struct {
	ID               int64
	ProviderCenterID int64
	ReceiverCenterID int64
	CostTypeID       int64
	Quantity         int64
	AmountCents      int64
	ChargedOn        time.Time
	ChargedBy        string
}

// NewServiceCharge validates and builds a charge, pricing it at the rate it was given. The amount
// is computed once, here, and then stored: a rate can change afterwards, and a charge that was
// made is a fact about the books rather than a view of today's prices.
func NewServiceCharge(providerCenterID, receiverCenterID, costTypeID, quantity, rateCents int64, chargedOn time.Time) (ServiceCharge, error) {
	if providerCenterID <= 0 || receiverCenterID <= 0 {
		return ServiceCharge{}, ErrCostCenterNotFound
	}
	if providerCenterID == receiverCenterID {
		return ServiceCharge{}, ErrServiceChargeSelf
	}
	if costTypeID <= 0 {
		return ServiceCharge{}, ErrCostTypeNotFound
	}
	if quantity <= 0 {
		return ServiceCharge{}, ErrQuantityNotPositive
	}
	if rateCents <= 0 {
		return ServiceCharge{}, ErrRateNotPositive
	}
	if rateCents > math.MaxInt64/quantity {
		return ServiceCharge{}, ErrChargeTooLarge
	}
	return ServiceCharge{
		ProviderCenterID: providerCenterID, ReceiverCenterID: receiverCenterID,
		CostTypeID: costTypeID, Quantity: quantity, AmountCents: rateCents * quantity,
		ChargedOn: chargedOn,
	}, nil
}

// DefineServiceRate validates and stores a rate for a cost type.
func (s *Service) DefineServiceRate(ctx context.Context, costTypeID, rateCents int64, validFrom time.Time, validTo *time.Time, by string) (ServiceRate, error) {
	if strings.TrimSpace(by) == "" {
		return ServiceRate{}, ErrActorRequired
	}
	rate, err := NewServiceRate(costTypeID, rateCents, validFrom, validTo)
	if err != nil {
		return ServiceRate{}, err
	}
	// The cost type must exist, so a typo is a clear not-found rather than a foreign-key failure.
	ct, err := s.repo.GetCostType(ctx, costTypeID)
	if err != nil {
		return ServiceRate{}, err
	}
	rate.CreatedBy = by
	saved, err := s.repo.CreateServiceRate(ctx, rate)
	if err != nil {
		return ServiceRate{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "service_rate.define",
		Subject: costTypeSubject(costTypeID),
		After: fmt.Sprintf("code=%s rate_cents=%d valid_from=%s",
			ct.Code, saved.RateCents, saved.ValidFrom.Format("2006-01-02")),
	}); err != nil {
		return ServiceRate{}, err
	}
	return saved, nil
}

// ServiceRates lists every rate, oldest first.
func (s *Service) ServiceRates(ctx context.Context) ([]ServiceRate, error) {
	return s.repo.ServiceRates(ctx)
}

// ChargeService charges one cost centre for another's service: the amount is the quantity priced
// at the rate in force on the day, the receiver is charged it and the provider is relieved by it.
// Both ends must exist and no rate in force means nothing to charge.
func (s *Service) ChargeService(ctx context.Context, providerCenterID, receiverCenterID, costTypeID, quantity int64, chargedOn time.Time, by string) (ServiceCharge, error) {
	if strings.TrimSpace(by) == "" {
		return ServiceCharge{}, ErrActorRequired
	}
	if _, err := s.repo.GetCostCenter(ctx, providerCenterID); err != nil {
		return ServiceCharge{}, err
	}
	receiver, err := s.repo.GetCostCenter(ctx, receiverCenterID)
	if err != nil {
		return ServiceCharge{}, err
	}
	rate, err := s.repo.RateFor(ctx, costTypeID, chargedOn)
	if err != nil {
		return ServiceCharge{}, err
	}
	charge, err := NewServiceCharge(providerCenterID, receiverCenterID, costTypeID, quantity, rate.RateCents, chargedOn)
	if err != nil {
		return ServiceCharge{}, err
	}
	charge.ChargedBy = by

	// The provider is relieved by exactly what the receiver is charged, so a charge moves cost
	// between centres without creating or destroying any.
	cost, err := NewRecord(receiverCenterID, nil, nil, Cost, charge.AmountCents, chargedOn, "")
	if err != nil {
		return ServiceCharge{}, err
	}
	cost.BookedBy = by
	relief, err := NewRecord(providerCenterID, nil, nil, Revenue, charge.AmountCents, chargedOn, "")
	if err != nil {
		return ServiceCharge{}, err
	}
	relief.BookedBy = by

	saved, err := s.repo.RecordServiceCharge(ctx, charge, []Record{cost, relief})
	if err != nil {
		return ServiceCharge{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "service_charge.record",
		Subject: costCenterSubject(providerCenterID),
		After: fmt.Sprintf("receiver=%s cost_type=%d quantity=%d amount_cents=%d charged_on=%s",
			receiver.Number, saved.CostTypeID, saved.Quantity, saved.AmountCents,
			saved.ChargedOn.Format("2006-01-02")),
	}); err != nil {
		return ServiceCharge{}, err
	}
	return saved, nil
}
