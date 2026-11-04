package klr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// CostUnitKey is a distribution key from a cost centre to a cost unit (Kostenträger): how much
// of the centre's costs the unit takes.
type CostUnitKey struct {
	ID             int64
	SourceCenterID int64
	TargetUnitID   int64
	ShareBP        int64
	ValidFrom      time.Time
	ValidTo        *time.Time
	CreatedBy      string
}

// NewCostUnitKey validates and builds a key. The share and period rules are the same as the
// centre-side keys', so they are reported with the same errors rather than a second set.
func NewCostUnitKey(sourceCenterID, targetUnitID, shareBP int64, validFrom time.Time, validTo *time.Time) (CostUnitKey, error) {
	if sourceCenterID <= 0 {
		return CostUnitKey{}, ErrCostCenterNotFound
	}
	if targetUnitID <= 0 {
		return CostUnitKey{}, ErrCostUnitNotFound
	}
	if shareBP < 0 || shareBP > 10000 {
		return CostUnitKey{}, ErrAllocationShare
	}
	if validTo != nil && validTo.Before(validFrom) {
		return CostUnitKey{}, ErrAllocationPeriod
	}
	return CostUnitKey{
		SourceCenterID: sourceCenterID, TargetUnitID: targetUnitID,
		ShareBP: shareBP, ValidFrom: validFrom, ValidTo: validTo,
	}, nil
}

// Active reports whether the key applies at a point in time. A key with no end date is
// open-ended; the whole validity range is inclusive.
func (k CostUnitKey) Active(at time.Time) bool {
	if at.Before(k.ValidFrom) {
		return false
	}
	return k.ValidTo == nil || !at.After(*k.ValidTo)
}

// String renders a key for messages and logs.
func (k CostUnitKey) String() string {
	return fmt.Sprintf("cost unit key %d: centre %d -> unit %d at %d bp", k.ID, k.SourceCenterID, k.TargetUnitID, k.ShareBP)
}

// UnitShare is one cost unit's slice of a Kostenträger run.
type UnitShare struct {
	Key   CostUnitKey
	Cents int64
}

// SplitUnitShares divides amountCents over cost-unit keys in basis points, on the same rule as
// the centre-side split: every key but the last is rounded down and the last takes the remainder,
// so the slices add up to the amount exactly.
func SplitUnitShares(amountCents int64, keys []CostUnitKey) ([]UnitShare, error) {
	sharesBP := make([]int64, 0, len(keys))
	for _, k := range keys {
		sharesBP = append(sharesBP, k.ShareBP)
	}
	cents, err := splitByBasisPoints(amountCents, sharesBP)
	if err != nil {
		return nil, err
	}
	out := make([]UnitShare, 0, len(keys))
	for i, k := range keys {
		out = append(out, UnitShare{Key: k, Cents: cents[i]})
	}
	return out, nil
}

// planCostUnits turns a period's costs and the keys in force into the records a run books: one
// cost on the source centre tagged with each unit that takes a share, and one revenue relieving
// the source by everything it handed out.
//
// The source centre keeps the same balance it had — the tagged costs and the relief cancel — so
// the centre is not distorted by attributing its costs, and the unit carries them. Records are
// append-only, so the cost *volume* rises by the attributed amount; the balances do not.
func planCostUnits(sourceCenterID, amountCents int64, keys []CostUnitKey, businessDate time.Time, by string) ([]Record, error) {
	shares, err := SplitUnitShares(amountCents, keys)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(shares)+1)
	var moved int64
	for _, share := range shares {
		if share.Cents == 0 {
			// A record's amount must be positive, so a share that rounds to nothing books
			// nothing. It moves nothing either way.
			continue
		}
		unitID := share.Key.TargetUnitID
		rec, err := NewRecord(sourceCenterID, &unitID, nil, Cost, share.Cents, businessDate, "")
		if err != nil {
			return nil, err
		}
		rec.BookedBy = by
		records = append(records, rec)
		moved += share.Cents
	}
	relief, err := NewRecord(sourceCenterID, nil, nil, Revenue, moved, businessDate, "")
	if err != nil {
		return nil, err
	}
	relief.BookedBy = by
	return append(records, relief), nil
}

// DefineCostUnitKey validates and stores a Kostenträger key.
func (s *Service) DefineCostUnitKey(ctx context.Context, sourceCenterID, targetUnitID, shareBP int64, validFrom time.Time, validTo *time.Time, by string) (CostUnitKey, error) {
	if strings.TrimSpace(by) == "" {
		return CostUnitKey{}, ErrActorRequired
	}
	key, err := NewCostUnitKey(sourceCenterID, targetUnitID, shareBP, validFrom, validTo)
	if err != nil {
		return CostUnitKey{}, err
	}
	// Both ends must exist, so a typo is a clear not-found rather than a foreign-key failure.
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return CostUnitKey{}, err
	}
	unit, err := s.repo.GetCostUnit(ctx, targetUnitID)
	if err != nil {
		return CostUnitKey{}, err
	}
	key.CreatedBy = by
	saved, err := s.repo.CreateCostUnitKey(ctx, key)
	if err != nil {
		return CostUnitKey{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_unit_key.define",
		Subject: costCenterSubject(sourceCenterID),
		After: fmt.Sprintf("target_unit=%s share_bp=%d valid_from=%s",
			unit.Number, saved.ShareBP, saved.ValidFrom.Format("2006-01-02")),
	}); err != nil {
		return CostUnitKey{}, err
	}
	return saved, nil
}

// CostUnitKeys lists a cost centre's Kostenträger keys. An unknown centre is a not-found rather
// than an empty list, exactly as reading its report is.
func (s *Service) CostUnitKeys(ctx context.Context, sourceCenterID int64) ([]CostUnitKey, error) {
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return nil, err
	}
	return s.repo.CostUnitKeys(ctx, sourceCenterID)
}

// AllocateToCostUnits runs the Kostenträgerrechnung for one cost centre over a period: the costs
// booked on it in that period are attributed to its cost units in the shares its keys give, and
// the source is relieved by the same amount. A second run over the same period is refused — the
// costs must not be attributed twice — and so is a run over a period with no costs.
func (s *Service) AllocateToCostUnits(ctx context.Context, sourceCenterID int64, from, to time.Time, by string) (Run, error) {
	if strings.TrimSpace(by) == "" {
		return Run{}, ErrActorRequired
	}
	if to.Before(from) {
		return Run{}, ErrAllocationRunPeriod
	}
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return Run{}, err
	}

	keys, err := s.repo.CostUnitKeys(ctx, sourceCenterID)
	if err != nil {
		return Run{}, err
	}
	// The keys in force at the end of the period are the ones that distribute it.
	eligible := make([]CostUnitKey, 0, len(keys))
	for _, k := range keys {
		if k.Active(to) {
			eligible = append(eligible, k)
		}
	}
	if len(eligible) == 0 {
		return Run{}, ErrAllocationRunNoTargets
	}

	amount, err := s.repo.CostsInPeriod(ctx, sourceCenterID, from, to)
	if err != nil {
		return Run{}, err
	}
	// A period with no costs in it is refused by the split itself (ErrNothingToAllocate) rather
	// than by a second check here, so the rule lives in one place.

	// The records a run writes belong to the period it distributed, not to the day it ran.
	records, err := planCostUnits(sourceCenterID, amount, eligible, to, by)
	if err != nil {
		return Run{}, err
	}
	run, err := s.repo.RecordCostUnitRun(ctx, Run{
		SourceCenterID: sourceCenterID,
		PeriodFrom:     from,
		PeriodTo:       to,
		AllocatedCents: amount,
		Receivers:      len(records) - 1,
		RunBy:          by,
	}, records)
	if err != nil {
		return Run{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_unit_run.run",
		Subject: costCenterSubject(sourceCenterID),
		After: fmt.Sprintf("period=%s..%s allocated_cents=%d receivers=%d",
			from.Format("2006-01-02"), to.Format("2006-01-02"), run.AllocatedCents, run.Receivers),
	}); err != nil {
		return Run{}, err
	}
	return run, nil
}
