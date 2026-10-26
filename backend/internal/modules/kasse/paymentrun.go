package kasse

import (
	"errors"
	"strings"
	"time"
)

// Slice S3: payment run (Zahlungslauf) and SEPA credit-transfer export.
//
// A run collects approved payment orders for one debtor cash account and
// produces one ISO 20022 pain.001.001.09 file. The produced bytes are stored, so
// the run is reproducible and an approved order can be paid at most once.

var (
	// ErrRunReferenceRequired is returned when a run has no reference.
	ErrRunReferenceRequired = errors.New("kasse: a run reference is required")
	// ErrRunReferenceExists is returned when the reference is already in use.
	ErrRunReferenceExists = errors.New("kasse: a run with that reference already exists")
	// ErrNoOrders is returned when a run selects no orders.
	ErrNoOrders = errors.New("kasse: a run needs at least one order")
	// ErrOrderNotApproved is returned when a selected order is not approved.
	ErrOrderNotApproved = errors.New("kasse: only approved orders can be paid")
	// ErrOrderAlreadyPaid is returned when an order already belongs to a run.
	ErrOrderAlreadyPaid = errors.New("kasse: the order is already part of a payment run")
	// ErrCashAccountRequired is returned when no debtor account is given.
	ErrCashAccountRequired = errors.New("kasse: a cash account is required")
	// ErrExecutionDateRequired is returned when the execution date is missing.
	ErrExecutionDateRequired = errors.New("kasse: an execution date is required")
	// ErrRunNotFound is returned when a run does not exist.
	ErrRunNotFound = errors.New("kasse: payment run not found")
	// ErrBICInvalid is returned for a BIC that is not a valid ISO 9362 code.
	ErrBICInvalid = errors.New("kasse: invalid BIC")
)

// CashAccount is one of the university's own accounts (the debtor side).
type CashAccount struct {
	ID       int64
	Name     string
	IBAN     string
	BIC      string
	Currency string
}

// NewCashAccount validates and builds a cash account.
func NewCashAccount(name, iban, bic, currency string) (CashAccount, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return CashAccount{}, ErrRecipientRequired
	}
	if !ValidIBAN(iban) {
		return CashAccount{}, ErrIBANInvalid
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = "EUR"
	}
	if len(currency) != 3 {
		return CashAccount{}, ErrBadCurrency
	}
	return CashAccount{Name: name, IBAN: NormalizeIBAN(iban), BIC: strings.ToUpper(strings.TrimSpace(bic)), Currency: currency}, nil
}

// PaymentRun is one produced bank file for one debtor account.
type PaymentRun struct {
	ID            int64
	Reference     string
	AccountID     int64
	ExecutionDate time.Time
	MessageID     string
	ItemCount     int
	SumCents      int64
	FileHash      string
	CreatedBy     string
}

// PaymentRunItem links a run to the order it pays.
type PaymentRunItem struct {
	OrderID     int64
	EndToEndID  string
	AmountCents int64
}

// PayableOrder is the projection of an order the run needs to build the file.
type PayableOrder struct {
	ID           int64
	Reference    string
	AmountCents  int64
	Currency     string
	Purpose      string
	Status       OrderStatus
	CreditorName string
	CreditorIBAN string
	CreditorBIC  string
}
