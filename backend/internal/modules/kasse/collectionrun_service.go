package kasse

import (
	"context"
	"fmt"
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

// CollectionRunService implements the SEPA direct-debit collection run (issue #22).
type CollectionRunService struct {
	repo     CollectionRunRepository
	cash     cashAccountReader
	mandates mandateReader
	audit    AuditSink
}

// NewCollectionRunService wires the collection-run service.
func NewCollectionRunService(repo CollectionRunRepository, cash cashAccountReader, mandates mandateReader, sink AuditSink) *CollectionRunService {
	return &CollectionRunService{repo: repo, cash: cash, mandates: mandates, audit: sink}
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

	transactions := make([]sepa.DirectDebitTransaction, 0, len(reqItems))
	for _, it := range reqItems {
		m, err := s.mandates.GetMandate(ctx, it.MandateReference)
		if err != nil {
			return CollectionRun{}, err
		}
		if !m.Active {
			return CollectionRun{}, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateNotActive)
		}
		if m.SequenceType != sequenceType {
			return CollectionRun{}, fmt.Errorf("kasse: mandate %q: %w", m.Reference, ErrMandateSequenceMismatch)
		}
		transactions = append(transactions, sepa.DirectDebitTransaction{
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

	dd := sepa.DirectDebit{
		MessageID:       ref,
		CreationTime:    nowFunc(),
		InitiatingParty: account.Name,
		CreditorName:    account.Name,
		CreditorIBAN:    account.IBAN,
		CreditorBIC:     account.BIC,
		CreditorID:      creditorID,
		SequenceType:    string(sequenceType),
		LocalInstrument: localInstrument,
		CollectionDate:  collectionDate,
		Currency:        account.Currency,
		Transactions:    transactions,
	}
	fileXML, err := dd.Marshal()
	if err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: build SEPA direct-debit file: %w", err)
	}

	run := CollectionRun{
		Reference:       ref,
		AccountID:       account.ID,
		CreditorID:      creditorID,
		CollectionDate:  collectionDate,
		SequenceType:    sequenceType,
		LocalInstrument: localInstrument,
		MessageID:       ref,
		ItemCount:       len(transactions),
		SumCents:        dd.Sum(),
		FileHash:        hashFile(fileXML),
		CreatedBy:       by,
	}
	saved, err := s.repo.InsertCollectionRun(ctx, run, fileXML, reqItems)
	if err != nil {
		return CollectionRun{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "collection_run.create",
		Subject: collectionRunSubject(saved.ID),
		After:   fmt.Sprintf("reference=%s items=%d sum_cents=%d hash=%s", saved.Reference, saved.ItemCount, saved.SumCents, saved.FileHash),
	}); err != nil {
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

func collectionRunSubject(id int64) string { return fmt.Sprintf("direct_debit_run/%d", id) }
