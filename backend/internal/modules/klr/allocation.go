package klr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// plan turns a period's costs and the keys in force into the records a run books: one cost on
// each receiver for its share, and one revenue on the source for everything it handed out. The
// engine's arithmetic lives here on its own, so it can be read and tested without a database.
//
// The relief equals exactly what the receivers were charged (SplitShares guarantees the shares
// add up to the amount and only a share that rounds to nothing is skipped, which moves nothing),
// so a run redistributes the KLR's costs without changing their total.
func plan(sourceCenterID, amountCents int64, keys []Allocation, businessDate time.Time, by string) ([]Record, error) {
	shares, err := SplitShares(amountCents, keys)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(shares))
	var moved int64
	for _, share := range shares {
		if share.Cents == 0 {
			// A record's amount must be positive, so a share that rounds to nothing books
			// nothing. It moves nothing either way.
			continue
		}
		rec, err := NewRecord(share.Allocation.TargetCenterID, nil, Cost, share.Cents, businessDate, "")
		if err != nil {
			return nil, err
		}
		rec.BookedBy = by
		records = append(records, rec)
		moved += share.Cents
	}
	relief, err := NewRecord(sourceCenterID, nil, Revenue, moved, businessDate, "")
	if err != nil {
		return nil, err
	}
	relief.BookedBy = by
	return append(records, relief), nil
}

// DefineAllocation validates and stores a distribution key.
func (s *Service) DefineAllocation(ctx context.Context, sourceCenterID, targetCenterID, shareBP int64, validFrom time.Time, validTo *time.Time, by string) (Allocation, error) {
	if strings.TrimSpace(by) == "" {
		return Allocation{}, ErrActorRequired
	}
	key, err := NewAllocation(sourceCenterID, targetCenterID, shareBP, validFrom, validTo)
	if err != nil {
		return Allocation{}, err
	}
	// Both ends must exist, so a typo is a clear not-found rather than a foreign-key failure.
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return Allocation{}, err
	}
	target, err := s.repo.GetCostCenter(ctx, targetCenterID)
	if err != nil {
		return Allocation{}, err
	}
	key.CreatedBy = by
	saved, err := s.repo.CreateAllocation(ctx, key)
	if err != nil {
		return Allocation{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "allocation.define",
		Subject: costCenterSubject(sourceCenterID),
		After: fmt.Sprintf("target=%s share_bp=%d valid_from=%s",
			target.Number, saved.ShareBP, saved.ValidFrom.Format("2006-01-02")),
	}); err != nil {
		return Allocation{}, err
	}
	return saved, nil
}

// Allocations lists a cost centre's distribution keys. An unknown centre is a not-found rather
// than an empty list, exactly as reading its report is.
func (s *Service) Allocations(ctx context.Context, sourceCenterID int64) ([]Allocation, error) {
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return nil, err
	}
	return s.repo.Allocations(ctx, sourceCenterID)
}

// Allocate runs the distribution for one cost centre over a period: the costs booked on it in
// that period are handed to its receivers in the shares its keys give, and the source is
// relieved by the same amount. A second run over the same period is refused — the costs must
// not be distributed twice — and so is a run over a period with no costs to distribute.
func (s *Service) Allocate(ctx context.Context, sourceCenterID int64, from, to time.Time, by string) (Run, error) {
	if strings.TrimSpace(by) == "" {
		return Run{}, ErrActorRequired
	}
	if to.Before(from) {
		return Run{}, ErrAllocationRunPeriod
	}
	if _, err := s.repo.GetCostCenter(ctx, sourceCenterID); err != nil {
		return Run{}, err
	}

	keys, err := s.repo.Allocations(ctx, sourceCenterID)
	if err != nil {
		return Run{}, err
	}
	// The keys in force at the end of the period are the ones that distribute it.
	eligible := make([]Allocation, 0, len(keys))
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
	records, err := plan(sourceCenterID, amount, eligible, to, by)
	if err != nil {
		return Run{}, err
	}
	run, err := s.repo.RecordRun(ctx, Run{
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
		Action:  "allocation.run",
		Subject: costCenterSubject(sourceCenterID),
		After: fmt.Sprintf("period=%s..%s allocated_cents=%d receivers=%d",
			from.Format("2006-01-02"), to.Format("2006-01-02"), run.AllocatedCents, run.Receivers),
	}); err != nil {
		return Run{}, err
	}
	return run, nil
}
