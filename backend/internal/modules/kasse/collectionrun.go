package kasse

import (
	"errors"
	"strings"
	"time"
)

// SEPA direct-debit collection run (issue #22): the collection counterpart of the
// payment run. It gathers mandates into one pain.008 file for one collection date
// and one sequence type, and stores the produced file so the run is reproducible.

var (
	// ErrNoCollectionItems is returned when a run selects no items.
	ErrNoCollectionItems = errors.New("kasse: a collection run needs at least one item")
	// ErrCollectionCreditorIDRequired is returned when no creditor identifier is given.
	ErrCollectionCreditorIDRequired = errors.New("kasse: a collection run needs a creditor identifier")
	// ErrCollectionDateRequired is returned when the collection date is missing.
	ErrCollectionDateRequired = errors.New("kasse: a collection date is required")
	// ErrMandateNotActive is returned when a selected mandate is inactive.
	ErrMandateNotActive = errors.New("kasse: the mandate is not active")
	// ErrMandateSequenceMismatch is returned when a mandate's sequence type differs
	// from the run's.
	ErrMandateSequenceMismatch = errors.New("kasse: mandate sequence type does not match the run")
	// ErrDuplicateMandate is returned when one mandate appears twice in a run.
	ErrDuplicateMandate = errors.New("kasse: the mandate appears twice in the run")
	// ErrCollectionRunNotFound is returned when a collection run does not exist.
	ErrCollectionRunNotFound = errors.New("kasse: collection run not found")
	// ErrLocalInstrumentUnknown is returned for a local instrument outside CORE/B2B.
	ErrLocalInstrumentUnknown = errors.New("kasse: local instrument must be CORE or B2B")
	// ErrEndToEndIDRequired is returned when a collection item has no end-to-end id.
	ErrEndToEndIDRequired = errors.New("kasse: an end-to-end id is required")
)

// validLocalInstrument reports whether s is a SEPA local instrument (CORE / B2B).
func validLocalInstrument(s string) bool {
	return s == "CORE" || s == "B2B"
}

// CollectionRun is one produced direct-debit file for one collection date.
type CollectionRun struct {
	ID              int64
	Reference       string
	AccountID       int64
	CreditorID      string
	CollectionDate  time.Time
	SequenceType    MandateSequence
	LocalInstrument string // CORE / B2B
	MessageID       string
	ItemCount       int
	SumCents        int64
	FileHash        string
	CreatedBy       string
}

// CollectionRunItem links a run to the mandate it collects from.
type CollectionRunItem struct {
	MandateReference string
	EndToEndID       string
	AmountCents      int64
	Purpose          string
}

// CollectionItem is one requested collection: a mandate plus the amount.
type CollectionItem struct {
	MandateReference string
	EndToEndID       string
	AmountCents      int64
	Purpose          string
}

// NewCollectionRun validates the run-level inputs (the per-item mandate checks
// happen in the service, where the mandates are loaded).
func NewCollectionRun(reference string, accountID int64, creditorID string, collectionDate time.Time,
	sequenceType MandateSequence, localInstrument string,
	items []CollectionItem) (string, []CollectionRunItem, error) {

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", nil, ErrRunReferenceRequired
	}
	if accountID <= 0 {
		return "", nil, ErrCashAccountRequired
	}
	if strings.TrimSpace(creditorID) == "" {
		return "", nil, ErrCollectionCreditorIDRequired
	}
	if collectionDate.IsZero() {
		return "", nil, ErrCollectionDateRequired
	}
	if !ValidSequenceType(sequenceType) {
		return "", nil, ErrSequenceTypeUnknown
	}
	if !validLocalInstrument(localInstrument) {
		return "", nil, ErrLocalInstrumentUnknown
	}
	if len(items) == 0 {
		return "", nil, ErrNoCollectionItems
	}
	seen := make(map[string]bool, len(items))
	out := make([]CollectionRunItem, 0, len(items))
	for _, it := range items {
		ref := strings.TrimSpace(it.MandateReference)
		if ref == "" {
			return "", nil, ErrMandateReferenceRequired
		}
		if seen[ref] {
			return "", nil, ErrDuplicateMandate
		}
		seen[ref] = true
		if it.AmountCents <= 0 {
			return "", nil, ErrAmountNotPositive
		}
		e2e := strings.TrimSpace(it.EndToEndID)
		if e2e == "" {
			return "", nil, ErrEndToEndIDRequired
		}
		out = append(out, CollectionRunItem{
			MandateReference: ref,
			EndToEndID:       e2e,
			AmountCents:      it.AmountCents,
			Purpose:          strings.TrimSpace(it.Purpose),
		})
	}
	return reference, out, nil
}
