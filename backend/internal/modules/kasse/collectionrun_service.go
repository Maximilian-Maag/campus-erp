package kasse

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/sepa"
)

// mandateReader loads a mandate by reference.
type mandateReader interface {
	GetMandate(ctx context.Context, reference string) (Mandate, error)
}

// cashAccountReader loads a cash account by id.
type cashAccountReader interface {
	GetCashAccount(ctx context.Context, id int64) (CashAccount, error)
}

// receivableReader selects the open items a run can gather.
type receivableReader interface {
	OpenReceivablesForCollection(ctx context.Context, dueBy time.Time, sequenceType MandateSequence) ([]Receivable, error)
}

// CollectionRunService implements the SEPA direct-debit collection run (issue #22).
type CollectionRunService struct {
	repo        CollectionRunRepository
	cash        cashAccountReader
	mandates    mandateReader
	receivables receivableReader
	audit       AuditSink
}

// NewCollectionRunService wires the collection-run service.
func NewCollectionRunService(repo CollectionRunRepository, cash cashAccountReader, mandates mandateReader, receivables receivableReader, sink AuditSink) *CollectionRunService {
	return &CollectionRunService{repo: repo, cash: cash, mandates: mandates, receivables: receivables, audit: sink}
}

// CreateCollectionRun gathers the given mandates into one pain.008 file for one
// collection date and sequence type. It is idempotent on the reference and fails
// closed on a blank/inactive mandate or a sequence-type mismatch.
func (s *CollectionRunService) CreateCollectionRun(ctx context.Context, reference string, accountID int64,
	creditorID string, collectionDate time.Time, sequenceType MandateSequence,
	localInstrument string, items []CollectionItem, by string) (CollectionRun, error) {

	if err := requireActor(by); err != nil {
		return CollectionRun{}, err
	}
	ref, reqItems, err := NewCollectionRun(reference, accountID, creditorID, collectionDate, sequenceType, localInstrument, items)
	if err != nil {
		return CollectionRun{}, err
	}
	if existing, ok, err := s.repo.CollectionRunByReference(ctx, ref); err != nil {
		return CollectionRun{}, err
	} else if ok {
		return existing, nil
	}

	account, err := s.cash.GetCashAccount(ctx, accountID)
	if err != nil {
		return CollectionRun{}, err
	}
	groups, err := s.resolveGroups(ctx, reqItems, sequenceType)
	if err != nil {
		return CollectionRun{}, err
	}
	run, fileXML, err := assembleRun(account, ref, creditorID, collectionDate, sequenceType, localInstrument, groups, by)
	if err != nil {
		return CollectionRun{}, err
	}
	saved, err := s.repo.InsertCollectionRun(ctx, run, fileXML, reqItems)
	if err != nil {
		return CollectionRun{}, err
	}
	if err := s.recordRun(ctx, saved, by); err != nil {
		return CollectionRun{}, err
	}
	return saved, nil
}

// CreateCollectionRunFromReceivables selects the open receivables due on or
// before dueBy, under active mandates matching the run's sequence type, and
// collects them: one collection per mandate, the mandate's items summed. The
// selected items are marked collected in the same transaction that stores the
// run, so a stored run and its claims never disagree. It is idempotent on the
// reference like the explicit run.
func (s *CollectionRunService) CreateCollectionRunFromReceivables(ctx context.Context, reference string,
	accountID int64, creditorID string, collectionDate time.Time, sequenceType MandateSequence,
	localInstrument string, dueBy time.Time, by string) (CollectionRun, error) {

	if err := requireActor(by); err != nil {
		return CollectionRun{}, err
	}
	ref, err := validateRunInputs(reference, accountID, creditorID, collectionDate, sequenceType, localInstrument)
	if err != nil {
		return CollectionRun{}, err
	}
	if dueBy.IsZero() {
		return CollectionRun{}, ErrReceivableDueByRequired
	}
	if dueBy.After(collectionDate) {
		return CollectionRun{}, ErrReceivableDueByAfterCollection
	}
	// Idempotent on the reference: re-issuing a run whose items are already
	// collected returns the stored run rather than finding nothing left to select.
	if existing, ok, err := s.repo.CollectionRunByReference(ctx, ref); err != nil {
		return CollectionRun{}, err
	} else if ok {
		return existing, nil
	}
	recs, err := s.receivables.OpenReceivablesForCollection(ctx, dueBy, sequenceType)
	if err != nil {
		return CollectionRun{}, err
	}
	if len(recs) == 0 {
		return CollectionRun{}, ErrNoCollectionItems
	}
	account, err := s.cash.GetCashAccount(ctx, accountID)
	if err != nil {
		return CollectionRun{}, err
	}
	for _, r := range recs {
		if r.Currency != account.Currency {
			return CollectionRun{}, fmt.Errorf("kasse: receivable %d: %w", r.ID, ErrReceivableCurrencyMismatch)
		}
	}
	items, claims := CollectionItemsFromReceivables(recs)
	_, reqItems, err := NewCollectionRun(ref, accountID, creditorID, collectionDate, sequenceType, localInstrument, items)
	if err != nil {
		return CollectionRun{}, err
	}
	groups, err := s.resolveGroups(ctx, reqItems, sequenceType)
	if err != nil {
		return CollectionRun{}, err
	}
	run, fileXML, err := assembleRun(account, ref, creditorID, collectionDate, sequenceType, localInstrument, groups, by)
	if err != nil {
		return CollectionRun{}, err
	}
	saved, err := s.repo.InsertCollectionRunFromReceivables(ctx, run, fileXML, reqItems, claimIDs(claims))
	if err != nil {
		return CollectionRun{}, err
	}
	if err := s.recordRun(ctx, saved, by); err != nil {
		return CollectionRun{}, err
	}
	return saved, nil
}

// CollectionRun loads a run by reference.
func (s *CollectionRunService) CollectionRun(ctx context.Context, reference string) (CollectionRun, error) {
	return s.repo.GetCollectionRun(ctx, reference)
}

// CollectionRunFile returns the stored pain.008 file of a run.
func (s *CollectionRunService) CollectionRunFile(ctx context.Context, reference string) (string, error) {
	return s.repo.CollectionRunFile(ctx, reference)
}

// mandateGroup is the transactions a run collects under one sequence type.
type mandateGroup struct {
	SequenceType MandateSequence
	Transactions []sepa.DirectDebitTransaction
}

// sequenceOrder is the order the instructions appear in a file: the SEPA lifecycle rather than the
// order the items arrived, so a first collection is read before the recurring ones it precedes.
var sequenceOrder = []MandateSequence{"FRST", "RCUR", "OOFF", "FNAL"}

// resolveGroups loads each item's mandate and groups its transaction by the sequence type it must be
// collected under, refusing an inactive mandate. With a run-level type every item must match it, so
// there is one group; with none, each item rides its mandate's own type and there is one group per
// type — the file then carries one payment instruction per type, which is why the type is not a
// message-level field.
func (s *CollectionRunService) resolveGroups(ctx context.Context, items []CollectionRunItem, runType MandateSequence) ([]mandateGroup, error) {
	byType := make(map[MandateSequence][]sepa.DirectDebitTransaction, len(sequenceOrder))
	for _, it := range items {
		m, err := s.mandates.GetMandate(ctx, it.MandateReference)
		if err != nil {
			return nil, err
		}
		if !m.Active {
			return nil, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateNotActive)
		}
		if runType != "" && m.SequenceType != runType {
			return nil, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateSequenceMismatch)
		}
		byType[m.SequenceType] = append(byType[m.SequenceType], sepa.DirectDebitTransaction{
			EndToEndID:  it.EndToEndID,
			AmountCents: it.AmountCents,
			MandateID:   m.Reference,
			MandateDate: m.SignatureDate,
			DebtorName:  m.DebtorName,
			DebtorIBAN:  m.DebtorIBAN,
			DebtorBIC:   m.DebtorBIC,
			Purpose:     it.Purpose,
		})
	}
	groups := make([]mandateGroup, 0, len(byType))
	for _, t := range sequenceOrder {
		if txs, ok := byType[t]; ok {
			groups = append(groups, mandateGroup{SequenceType: t, Transactions: txs})
			delete(byType, t)
		}
	}
	// Any type outside the lifecycle order is still collected rather than dropped: a mandate's type
	// is validated when it is written, so this is belt and braces, not a path a caller reaches.
	leftovers := make([]string, 0, len(byType))
	for t := range byType {
		leftovers = append(leftovers, string(t))
	}
	sort.Strings(leftovers)
	for _, t := range leftovers {
		groups = append(groups, mandateGroup{SequenceType: MandateSequence(t), Transactions: byType[MandateSequence(t)]})
	}
	return groups, nil
}

// recordRun appends the audit event for a stored run.
func (s *CollectionRunService) recordRun(ctx context.Context, saved CollectionRun, by string) error {
	_, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "collection_run.create",
		Subject: collectionRunSubject(saved.ID),
		After:   fmt.Sprintf("reference=%s items=%d sum_cents=%d hash=%s", saved.Reference, saved.ItemCount, saved.SumCents, saved.FileHash),
	})
	return err
}

// assembleRun builds the pain.008 file and the run record from the resolved groups: one payment
// instruction per sequence type, under a header that totals them.
func assembleRun(account CashAccount, ref, creditorID string, collectionDate time.Time,
	runType MandateSequence, localInstrument string,
	groups []mandateGroup, by string) (CollectionRun, string, error) {

	batch := sepa.DirectDebitBatch{
		MessageID:       ref,
		CreationTime:    nowFunc(),
		InitiatingParty: account.Name,
		CreditorName:    account.Name,
		CreditorIBAN:    account.IBAN,
		CreditorBIC:     account.BIC,
		CreditorID:      creditorID,
		LocalInstrument: localInstrument,
		Currency:        account.Currency,
		Groups:          make([]sepa.DirectDebitGroup, 0, len(groups)),
	}
	items := 0
	for _, g := range groups {
		// A run of one type keeps the instruction id it has always had — the reference — so its file
		// is the same bytes as before; several types need one id per instruction to be addressable.
		id := ref
		if len(groups) > 1 {
			id = ref + "-" + string(g.SequenceType)
		}
		batch.Groups = append(batch.Groups, sepa.DirectDebitGroup{
			InstructionID:  id,
			SequenceType:   string(g.SequenceType),
			CollectionDate: collectionDate,
			Transactions:   g.Transactions,
		})
		items += len(g.Transactions)
	}
	fileXML, err := batch.Marshal()
	if err != nil {
		return CollectionRun{}, "", fmt.Errorf("kasse: build SEPA direct-debit file: %w", err)
	}
	run := CollectionRun{
		Reference:       ref,
		AccountID:       account.ID,
		CreditorID:      creditorID,
		CollectionDate:  collectionDate,
		SequenceType:    runType,
		LocalInstrument: localInstrument,
		MessageID:       ref,
		ItemCount:       items,
		SumCents:        batch.Sum(),
		FileHash:        hashFile(fileXML),
		CreatedBy:       by,
	}
	return run, fileXML, nil
}

// claimIDs flattens the mandate-to-ids map the grouping returns into the ids the
// store marks collected.
func claimIDs(claims map[string][]int64) []int64 {
	ids := make([]int64, 0, len(claims))
	for _, group := range claims {
		ids = append(ids, group...)
	}
	return ids
}

func collectionRunSubject(id int64) string { return fmt.Sprintf("direct_debit_run/%d", id) }
