package kasse

import (
	"errors"
	"sort"
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

// validateRunInputs checks the run-level fields and returns the trimmed
// reference. It is separate from NewCollectionRun so a run that selects its items
// can validate its inputs — and look itself up for idempotency — before it knows
// which items exist.
func validateRunInputs(reference string, accountID int64, creditorID string, collectionDate time.Time,
	sequenceType MandateSequence, localInstrument string) (string, error) {

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", ErrRunReferenceRequired
	}
	if accountID <= 0 {
		return "", ErrCashAccountRequired
	}
	if strings.TrimSpace(creditorID) == "" {
		return "", ErrCollectionCreditorIDRequired
	}
	if collectionDate.IsZero() {
		return "", ErrCollectionDateRequired
	}
	if !ValidSequenceType(sequenceType) {
		return "", ErrSequenceTypeUnknown
	}
	if !validLocalInstrument(localInstrument) {
		return "", ErrLocalInstrumentUnknown
	}
	return reference, nil
}

// NewCollectionRun validates the run-level inputs (the per-item mandate checks
// happen in the service, where the mandates are loaded).
func NewCollectionRun(reference string, accountID int64, creditorID string, collectionDate time.Time,
	sequenceType MandateSequence, localInstrument string,
	items []CollectionItem) (string, []CollectionRunItem, error) {

	reference, err := validateRunInputs(reference, accountID, creditorID, collectionDate, sequenceType, localInstrument)
	if err != nil {
		return "", nil, err
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

// CollectionItemsFromReceivables groups the selected open receivables by their
// mandate into the items of one run: the pain.008 file allows one collection per
// mandate, so a mandate's open items are collected as a single amount. The
// end-to-end id is the mandate reference, which is unique within the run (the
// run-item table enforces one item per mandate). The returned map records which
// receivable ids each mandate contributed, so the store marks exactly those
// collected and no others.
func CollectionItemsFromReceivables(recs []Receivable) ([]CollectionItem, map[string][]int64) {
	type group struct {
		amount   int64
		purposes []string
		ids      []int64
	}
	byMandate := map[string]*group{}
	order := make([]string, 0, len(recs))
	for _, r := range recs {
		g, ok := byMandate[r.MandateReference]
		if !ok {
			g = &group{}
			byMandate[r.MandateReference] = g
			order = append(order, r.MandateReference)
		}
		g.amount += r.AmountCents
		g.purposes = append(g.purposes, r.Purpose)
		g.ids = append(g.ids, r.ID)
	}
	sort.Strings(order)
	items := make([]CollectionItem, 0, len(order))
	claims := make(map[string][]int64, len(order))
	for _, ref := range order {
		g := byMandate[ref]
		items = append(items, CollectionItem{
			MandateReference: ref,
			EndToEndID:       ref,
			AmountCents:      g.amount,
			Purpose:          joinPurposes(g.purposes),
		})
		claims[ref] = g.ids
	}
	return items, claims
}

// joinPurposes joins the distinct purposes in order, capped at the ISO Max140Text
// limit the remittance field allows so one long list of claims still renders.
func joinPurposes(purposes []string) string {
	seen := make(map[string]bool, len(purposes))
	kept := make([]string, 0, len(purposes))
	for _, p := range purposes {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		kept = append(kept, p)
	}
	return truncateRunes(strings.Join(kept, "; "), maxPurposeLen)
}

// truncateRunes caps s at max runes, so a multi-byte purpose is never cut mid
// character.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
