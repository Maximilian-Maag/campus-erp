// Package klr implements the Kosten- und Leistungsrechnung: cost centres, cost
// units (Kostentraeger) and append-only accounting records with a report.
package klr

import (
	"errors"
	"strings"
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
type Record struct {
	ID           int64
	CostCenterID int64
	CostUnitID   *int64
	Kind         Kind
	AmountCents  int64
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

// NewRecord validates and builds a posting.
func NewRecord(costCenterID int64, costUnitID *int64, kind Kind, amountCents int64, reference string) (Record, error) {
	if costCenterID <= 0 {
		return Record{}, ErrCostCenterNotFound
	}
	if kind != Cost && kind != Revenue {
		return Record{}, ErrUnknownKind
	}
	if amountCents <= 0 {
		return Record{}, ErrAmountNotPositive
	}
	return Record{
		CostCenterID: costCenterID, CostUnitID: costUnitID, Kind: kind,
		AmountCents: amountCents, Reference: strings.TrimSpace(reference),
	}, nil
}
