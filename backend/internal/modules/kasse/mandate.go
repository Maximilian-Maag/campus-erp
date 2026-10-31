package kasse

import (
	"errors"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/sepa"
)

// SEPA direct-debit mandate: the authorisation a debtor gives the university to
// collect by direct debit. It supplies the mandate reference, the mandate's
// signature date and the sequence type that the pain.008 generator needs
// (docs/20-sepa-direct-debit.md). Not a money movement itself, so no amount here.

var (
	// ErrMandateReferenceRequired is returned for a blank mandate reference.
	ErrMandateReferenceRequired = errors.New("kasse: a mandate reference is required")
	// ErrMandateReferenceTooLong is returned for a reference over 35 characters.
	ErrMandateReferenceTooLong = errors.New("kasse: a mandate reference must be at most 35 characters")
	// ErrMandateReferenceExists is returned when the reference is already in use.
	ErrMandateReferenceExists = errors.New("kasse: a mandate with that reference already exists")
	// ErrMandateNotFound is returned when a mandate does not exist.
	ErrMandateNotFound = errors.New("kasse: mandate not found")
	// ErrSignatureDateRequired is returned when the mandate has no signature date.
	ErrSignatureDateRequired = errors.New("kasse: a mandate signature date is required")
	// ErrSequenceTypeUnknown is returned for a sequence type outside the SEPA set.
	ErrSequenceTypeUnknown = errors.New("kasse: sequence type must be FRST, RCUR, OOFF or FNAL")
	// ErrDebtorBICInvalid is returned for a debtor BIC that is not an ISO 9362 code.
	ErrDebtorBICInvalid = errors.New("kasse: invalid debtor BIC")
)

// maxMandateRefLen matches the ISO 20022 Max35Text limit on MndtId.
const maxMandateRefLen = 35

// MandateSequence is the SEPA sequence type of a collection.
type MandateSequence string

const (
	// SequenceFirst collects the first time a mandate is used.
	SequenceFirst MandateSequence = "FRST"
	// SequenceRecurring collects again under an existing mandate.
	SequenceRecurring MandateSequence = "RCUR"
	// SequenceOneOff collects once from a one-off mandate.
	SequenceOneOff MandateSequence = "OOFF"
	// SequenceFinal is the last collection under a mandate.
	SequenceFinal MandateSequence = "FNAL"
)

// ValidSequenceType reports whether s is one of the four SEPA sequence types.
func ValidSequenceType(s MandateSequence) bool {
	switch s {
	case SequenceFirst, SequenceRecurring, SequenceOneOff, SequenceFinal:
		return true
	default:
		return false
	}
}

// Mandate is a SEPA direct-debit mandate (legacy: a mandate reference attached to
// a debtor).
type Mandate struct {
	ID            int64
	Reference     string
	DebtorName    string
	DebtorIBAN    string
	DebtorBIC     string
	SignatureDate time.Time
	SequenceType  MandateSequence
	Active        bool
}

// NewMandate validates and builds an active mandate.
func NewMandate(reference, debtorName, debtorIBAN, debtorBIC string, signatureDate time.Time, seq MandateSequence) (Mandate, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Mandate{}, ErrMandateReferenceRequired
	}
	if len(reference) > maxMandateRefLen {
		return Mandate{}, ErrMandateReferenceTooLong
	}
	debtorName = strings.TrimSpace(debtorName)
	if debtorName == "" {
		return Mandate{}, ErrNameRequired
	}
	if !ValidIBAN(debtorIBAN) {
		return Mandate{}, ErrIBANInvalid
	}
	bic := strings.ToUpper(strings.TrimSpace(debtorBIC))
	if bic != "" && !sepa.ValidBIC(bic) {
		return Mandate{}, ErrDebtorBICInvalid
	}
	if signatureDate.IsZero() {
		return Mandate{}, ErrSignatureDateRequired
	}
	if !ValidSequenceType(seq) {
		return Mandate{}, ErrSequenceTypeUnknown
	}
	return Mandate{
		Reference:     reference,
		DebtorName:    debtorName,
		DebtorIBAN:    NormalizeIBAN(debtorIBAN),
		DebtorBIC:     bic,
		SignatureDate: signatureDate,
		SequenceType:  seq,
		Active:        true,
	}, nil
}
