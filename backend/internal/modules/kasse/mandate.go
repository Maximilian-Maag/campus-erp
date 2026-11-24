package kasse

import (
	"errors"
	"fmt"
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
	// ErrMandateRetired is returned when a mandate is retired although it already is: a debtor can
	// revoke an authorisation once, and a second revocation is a mistake worth naming rather than a
	// silent no-op that reads like a success.
	ErrMandateRetired = errors.New("kasse: mandate is already retired")
	// ErrMandateAlreadyFinal is returned when a mandate is marked final although it already is: the
	// last collection has been announced, and announcing it twice is a mistake worth naming.
	ErrMandateAlreadyFinal = errors.New("kasse: mandate is already final")
	// ErrMandateNotYetCollected is returned when a mandate is marked final before its first
	// collection: FNAL announces the last collection under an authorisation, and one that has never
	// collected anything has no last collection to announce.
	ErrMandateNotYetCollected = errors.New("kasse: a mandate must be collected before its final collection")
	// ErrMandateOneOff is returned when a one-off mandate is marked final: OOFF is already a single
	// collection, so there is no further one to announce.
	ErrMandateOneOff = errors.New("kasse: a one-off mandate has no final collection")
	// ErrMandateSequenceChanged is returned when a mandate's sequence type moved between the read a
	// decision was taken on and the write that carried it out: another run or operator advanced it
	// first, and the decision is stale rather than applicable.
	ErrMandateSequenceChanged = errors.New("kasse: the mandate's sequence type changed meanwhile")
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

// Retire withdraws the authorisation: a mandate is soft state, so the row stays and the audit trail
// keeps the decision (ADR-0006). Retiring a mandate that is already retired is refused, because a
// silent no-op would read like a revocation that took effect.
func (m Mandate) Retire() (Mandate, error) {
	if !m.Active {
		return Mandate{}, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateRetired)
	}
	m.Active = false
	return m, nil
}

// AdvanceAfterCollection returns the mandate as it stands once a collection under it has been
// produced: the first collection under a FRST mandate is the only one that may be collected as
// FRST — the Direct Debit Rulebook requires RCUR for every later one — so the type moves to RCUR.
// A recurring, final or one-off mandate stays as it is; the bool reports whether the type moved.
func (m Mandate) AdvanceAfterCollection() (Mandate, bool) {
	if m.SequenceType != SequenceFirst {
		return m, false
	}
	m.SequenceType = SequenceRecurring
	return m, true
}

// MarkFinal returns the mandate as it stands once the operator asks for its last collection: a
// recurring mandate moves to FNAL, so the next run collects it as the final one. A first mandate is
// refused — its collection has not happened yet, so there is no last one to announce — and a
// one-off or already-final mandate is refused as a mistake worth naming rather than a silent no-op.
func (m Mandate) MarkFinal() (Mandate, error) {
	switch m.SequenceType {
	case SequenceRecurring:
		m.SequenceType = SequenceFinal
		return m, nil
	case SequenceFirst:
		return Mandate{}, ErrMandateNotYetCollected
	case SequenceOneOff:
		return Mandate{}, ErrMandateOneOff
	case SequenceFinal:
		return Mandate{}, ErrMandateAlreadyFinal
	default:
		return Mandate{}, ErrSequenceTypeUnknown
	}
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
