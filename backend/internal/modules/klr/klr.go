// Package klr implements the Kosten- und Leistungsrechnung: cost centres, cost
// units (Kostentraeger) and append-only accounting records with a report.
package klr

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Validation errors.
var (
	ErrNumberRequired     = errors.New("klr: a number is required")
	ErrNameRequired       = errors.New("klr: a name is required")
	ErrAmountNotPositive  = errors.New("klr: the amount must be positive")
	ErrUnknownKind        = errors.New("klr: unknown record kind")
	ErrActorRequired      = errors.New("klr: an actor is required")
	ErrCostCenterExists   = errors.New("klr: a cost centre with that number already exists")
	ErrCostCenterNotFound = errors.New("klr: cost centre not found")
	ErrCostUnitExists     = errors.New("klr: a cost unit with that number already exists")
	ErrCostUnitNotFound   = errors.New("klr: cost unit not found")

	// Allocation errors.
	ErrAllocationSelf         = errors.New("klr: a cost centre cannot distribute to itself")
	ErrAllocationShare        = errors.New("klr: a share must be between 0 and 10000 basis points")
	ErrAllocationPeriod       = errors.New("klr: the validity period ends before it starts")
	ErrAllocationRunPeriod    = errors.New("klr: the run period ends before it starts")
	ErrSharesIncomplete       = errors.New("klr: the allocation shares of a cost centre do not add up to 10000 basis points")
	ErrNothingToAllocate      = errors.New("klr: the cost centre has no costs in that period")
	ErrAlreadyAllocated       = errors.New("klr: the period was already allocated for that cost centre")
	ErrAllocationRunNoTargets = errors.New("klr: an allocation run needs at least one key")
	ErrBusinessDateRequired   = errors.New("klr: a business date is required")
)

// Kind distinguishes a cost from a revenue.
type Kind string

const (
	// Cost is an expense booked on the KLR (Kosten).
	Cost Kind = "cost"
	// Revenue is an income booked on the KLR (Leistung).
	Revenue Kind = "revenue"
)

// CostCenter is a cost centre (Kostenstelle).
type CostCenter struct {
	ID        int64
	Number    string
	Name      string
	CreatedBy string
}

// CostUnit is a cost unit (Kostentraeger).
type CostUnit struct {
	ID        int64
	Number    string
	Name      string
	CreatedBy string
}

// Record is one append-only KLR posting.
//
// BusinessDate is the period the record belongs to, which is not the day it was entered: costs
// for the month just closed are booked after it ended, and the distribution engine runs over
// the period the costs belong to (docs/13).
type Record struct {
	ID           int64
	CostCenterID int64
	CostUnitID   *int64
	CostTypeID   *int64
	Kind         Kind
	AmountCents  int64
	BusinessDate time.Time
	Reference    string
	BookedBy     string
}

// Report is the derived state of a cost centre.
type Report struct {
	CostCenterID  int64
	CostsCents    int64
	RevenuesCents int64
}

// BalanceCents is revenues minus costs.
func (r Report) BalanceCents() int64 { return r.RevenuesCents - r.CostsCents }

func requireNumberName(number, name string, errNumber, errName error) (string, string, error) {
	number = strings.TrimSpace(number)
	if number == "" {
		return "", "", errNumber
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", errName
	}
	return number, name, nil
}

// NewCostCenter validates and builds a cost centre.
func NewCostCenter(number, name string) (CostCenter, error) {
	n, nm, err := requireNumberName(number, name, ErrNumberRequired, ErrNameRequired)
	if err != nil {
		return CostCenter{}, err
	}
	return CostCenter{Number: n, Name: nm}, nil
}

// NewCostUnit validates and builds a cost unit.
func NewCostUnit(number, name string) (CostUnit, error) {
	n, nm, err := requireNumberName(number, name, ErrNumberRequired, ErrNameRequired)
	if err != nil {
		return CostUnit{}, err
	}
	return CostUnit{Number: n, Name: nm}, nil
}

// NewRecord validates and builds a posting. The business date is required: a record has to
// belong to a period, and "whenever it was typed in" is not one. The cost unit and the cost type
// are optional — a record may be booked before either is known — and the store's foreign keys
// refuse one that does not exist.
func NewRecord(costCenterID int64, costUnitID, costTypeID *int64, kind Kind, amountCents int64, businessDate time.Time, reference string) (Record, error) {
	if costCenterID <= 0 {
		return Record{}, ErrCostCenterNotFound
	}
	if kind != Cost && kind != Revenue {
		return Record{}, ErrUnknownKind
	}
	if amountCents <= 0 {
		return Record{}, ErrAmountNotPositive
	}
	if businessDate.IsZero() {
		return Record{}, ErrBusinessDateRequired
	}
	return Record{
		CostCenterID: costCenterID, CostUnitID: costUnitID, CostTypeID: costTypeID, Kind: kind,
		AmountCents: amountCents, BusinessDate: businessDate, Reference: strings.TrimSpace(reference),
	}, nil
}

// Allocation is one distribution key (Verteilerschluessel): a share of a cost
// centre's costs that goes to another cost centre. ShareBP is in basis points
// (10000 = all of it). Distributing to a cost unit (Kostentraeger) is a separate
// step (docs/13).
type Allocation struct {
	ID             int64
	SourceCenterID int64
	TargetCenterID int64
	ShareBP        int64
	ValidFrom      time.Time
	ValidTo        *time.Time
	CreatedBy      string
}

// Run is one execution of the distribution engine over a period.
type Run struct {
	ID             int64
	SourceCenterID int64
	PeriodFrom     time.Time
	PeriodTo       time.Time
	AllocatedCents int64
	Receivers      int
	RunBy          string
}

// NewAllocation validates and builds a distribution key.
func NewAllocation(sourceCenterID, targetCenterID, shareBP int64, validFrom time.Time, validTo *time.Time) (Allocation, error) {
	if sourceCenterID <= 0 || targetCenterID <= 0 {
		return Allocation{}, ErrCostCenterNotFound
	}
	if targetCenterID == sourceCenterID {
		return Allocation{}, ErrAllocationSelf
	}
	if shareBP < 0 || shareBP > 10000 {
		return Allocation{}, ErrAllocationShare
	}
	if validTo != nil && validTo.Before(validFrom) {
		return Allocation{}, ErrAllocationPeriod
	}
	return Allocation{
		SourceCenterID: sourceCenterID, TargetCenterID: targetCenterID,
		ShareBP: shareBP, ValidFrom: validFrom, ValidTo: validTo,
	}, nil
}

// Active reports whether the key applies at a point in time. A key with no end
// date is open-ended; the whole validity range is inclusive.
func (a Allocation) Active(at time.Time) bool {
	if at.Before(a.ValidFrom) {
		return false
	}
	return a.ValidTo == nil || !at.After(*a.ValidTo)
}

// Share is one receiver's slice of an allocation run.
type Share struct {
	Allocation Allocation
	Cents      int64
}

// SplitShares divides amountCents over keys in basis points. Every key but the
// last is rounded down and the last takes the remainder, so the slices always
// add up to the amount exactly - no cent is created or lost. The keys must add
// up to 10000 basis points; they are split in the order given.
func SplitShares(amountCents int64, keys []Allocation) ([]Share, error) {
	if len(keys) == 0 {
		return nil, ErrAllocationRunNoTargets
	}
	if amountCents <= 0 {
		return nil, ErrNothingToAllocate
	}
	var total int64
	for _, k := range keys {
		total += k.ShareBP
	}
	if total != 10000 {
		return nil, ErrSharesIncomplete
	}
	shares := make([]Share, 0, len(keys))
	var given int64
	for i, k := range keys {
		var cents int64
		if i == len(keys)-1 {
			cents = amountCents - given
		} else {
			cents = amountCents * k.ShareBP / 10000
		}
		given += cents
		shares = append(shares, Share{Allocation: k, Cents: cents})
	}
	return shares, nil
}

// String renders a key for messages and logs.
func (a Allocation) String() string {
	return fmt.Sprintf("allocation %d: centre %d -> centre %d at %d bp", a.ID, a.SourceCenterID, a.TargetCenterID, a.ShareBP)
}
