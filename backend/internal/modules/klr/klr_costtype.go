package klr

import (
	"errors"
	"strings"
)

// Cost-type errors.
var (
	// ErrCostTypeExists is returned when the code is already in use.
	ErrCostTypeExists = errors.New("klr: a cost type with that code already exists")
	// ErrCostTypeNotFound is returned when a cost type does not exist.
	ErrCostTypeNotFound = errors.New("klr: cost type not found")
	// ErrCodeRequired is returned for a blank code.
	ErrCodeRequired = errors.New("klr: a code is required")
	// ErrAccountCodeRequired is returned for a blank account code on the bridge.
	ErrAccountCodeRequired = errors.New("klr: an account code is required")
	// ErrAccountAlreadyMapped is returned when an account code is already mapped to a different
	// cost type.
	ErrAccountAlreadyMapped = errors.New("klr: the account is already mapped to another cost type")
	// ErrAccountNotMapped is returned when no cost type is mapped to an account code.
	ErrAccountNotMapped = errors.New("klr: no cost type is mapped to that account")
)

// CostType is a cost type (Kostenart): what a record's cost is.
type CostType struct {
	ID        int64
	Code      string
	Name      string
	CreatedBy string
}

// NewCostType validates and builds a cost type.
func NewCostType(code, name string) (CostType, error) {
	c, nm, err := requireNumberName(code, name, ErrCodeRequired, ErrNameRequired)
	if err != nil {
		return CostType{}, err
	}
	return CostType{Code: c, Name: nm}, nil
}

// CostTypeAccount is one link of the bridge (Kontenüberleitung): a general-ledger account code
// that belongs to a cost type. The code is a string, not a foreign key — the ledger side is a
// later phase, so the bridge is data that will line up with it (docs/13).
type CostTypeAccount struct {
	ID          int64
	CostTypeID  int64
	AccountCode string
}

// NewCostTypeAccount validates and builds a bridge link.
func NewCostTypeAccount(costTypeID int64, accountCode string) (CostTypeAccount, error) {
	if costTypeID <= 0 {
		return CostTypeAccount{}, ErrCostTypeNotFound
	}
	code := strings.TrimSpace(accountCode)
	if code == "" {
		return CostTypeAccount{}, ErrAccountCodeRequired
	}
	return CostTypeAccount{CostTypeID: costTypeID, AccountCode: code}, nil
}
