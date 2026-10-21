package kasse

import (
	"errors"
	"strings"
	"time"
)

var (
	// ErrNoDunningBand is returned when no band covers the outstanding amount.
	ErrNoDunningBand = errors.New("kasse: no dunning band covers the amount")
	// ErrLevelCodeRequired is returned when a dunning level has no code.
	ErrLevelCodeRequired = errors.New("kasse: a dunning level code is required")
	// ErrBandsRequired is returned when a dunning level has no bands.
	ErrBandsRequired = errors.New("kasse: a dunning level needs at least one band")
	// ErrBandsNotAscending is returned when bands are not ordered by upper bound.
	ErrBandsNotAscending = errors.New("kasse: dunning bands must ascend by upper bound")
	// ErrNegativeBand is returned for negative band values.
	ErrNegativeBand = errors.New("kasse: dunning band values must not be negative")
	// ErrLevelInactive is returned when an inactive level is used.
	ErrLevelInactive = errors.New("kasse: the dunning level is inactive")
	// ErrClaimRequired is returned when a notice has no claim reference.
	ErrClaimRequired = errors.New("kasse: a claim reference is required")
	// ErrOpenBandNotLast is returned when an open-ended band is not the last one.
	ErrOpenBandNotLast = errors.New("kasse: an open-ended band must be the last band")
	// ErrLevelExists is returned when the (year, code) pair is already taken.
	ErrLevelExists = errors.New("kasse: a dunning level with that year and code already exists")
)

// DunningBand is one amount band of a dunning level: an upper bound, a fixed fee
// and a rate in basis points (1/100 of a percent). An upper bound of 0 is
// open-ended and matches any amount.
type DunningBand struct {
	UpToCents       int64
	FeeCents        int64
	RateBasisPoints int64
}

// DunningLevel is a dunning stage (legacy kbs.mahn).
type DunningLevel struct {
	ID          int64
	Year        int
	Code        string
	Description string
	DaysAllowed int
	Active      bool
	NextCode    string
	Bands       []DunningBand
}

// Validate checks the level and its bands.
func (l DunningLevel) Validate() error {
	if strings.TrimSpace(l.Code) == "" {
		return ErrLevelCodeRequired
	}
	if len(l.Bands) == 0 {
		return ErrBandsRequired
	}
	if l.DaysAllowed < 0 {
		return ErrNegativeBand
	}
	prev := int64(-1)
	for i, b := range l.Bands {
		if b.UpToCents < 0 || b.FeeCents < 0 || b.RateBasisPoints < 0 {
			return ErrNegativeBand
		}
		if b.UpToCents == 0 {
			// An open-ended band matches everything, so it must come last or it
			// would shadow every band behind it.
			if i != len(l.Bands)-1 {
				return ErrOpenBandNotLast
			}
			continue
		}
		if b.UpToCents <= prev {
			return ErrBandsNotAscending
		}
		prev = b.UpToCents
	}
	return nil
}

// SelectBand picks the band covering an outstanding amount: the first band whose
// upper bound is at least the amount, or an open-ended band.
func (l DunningLevel) SelectBand(amountCents int64) (DunningBand, error) {
	if amountCents <= 0 {
		return DunningBand{}, ErrAmountNotPositive
	}
	for _, b := range l.Bands {
		if b.UpToCents == 0 || amountCents <= b.UpToCents {
			return b, nil
		}
	}
	return DunningBand{}, ErrNoDunningBand
}

// Charge is the dunning charge for an outstanding amount: the band's fixed fee plus
// its rate applied to the amount, rounded half up. Integer arithmetic only, so the
// result never depends on floating-point rounding.
func (l DunningLevel) Charge(amountCents int64) (int64, error) {
	b, err := l.SelectBand(amountCents)
	if err != nil {
		return 0, err
	}
	return b.FeeCents + roundDiv(amountCents*b.RateBasisPoints, 10000), nil
}

// DueDate is when the level becomes actionable, counted from a reference date.
func (l DunningLevel) DueDate(from time.Time) time.Time {
	return from.AddDate(0, 0, l.DaysAllowed)
}

// roundDiv divides n by div, rounding half up. Callers pass non-negative values
// only (an amount times a basis-point rate), so no sign handling is needed.
func roundDiv(n, div int64) int64 {
	return (n + div/2) / div
}

// DunningNotice is an issued dunning notice for a claim.
type DunningNotice struct {
	ID               int64
	ClaimReference   string
	LevelCode        string
	OutstandingCents int64
	ChargeCents      int64
	IssuedOn         time.Time
	IssuedBy         string
}

// NewDunningNotice validates a notice and computes its charge from the level.
func NewDunningNotice(level DunningLevel, claimReference string, outstandingCents int64,
	issuedOn time.Time, issuedBy string) (DunningNotice, error) {

	claimReference = strings.TrimSpace(claimReference)
	if claimReference == "" {
		return DunningNotice{}, ErrClaimRequired
	}
	if strings.TrimSpace(issuedBy) == "" {
		return DunningNotice{}, ErrActorRequired
	}
	if !level.Active {
		return DunningNotice{}, ErrLevelInactive
	}
	charge, err := level.Charge(outstandingCents)
	if err != nil {
		return DunningNotice{}, err
	}
	return DunningNotice{
		ClaimReference:   claimReference,
		LevelCode:        level.Code,
		OutstandingCents: outstandingCents,
		ChargeCents:      charge,
		IssuedOn:         issuedOn,
		IssuedBy:         issuedBy,
	}, nil
}
