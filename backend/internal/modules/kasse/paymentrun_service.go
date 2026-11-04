package kasse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/sepa"
)

// PaymentRunService implements the payment-run / SEPA-export slice (S3).
type PaymentRunService struct {
	repo  PaymentRunRepository
	audit AuditSink
}

// NewPaymentRunService wires the payment-run service.
func NewPaymentRunService(repo PaymentRunRepository, sink AuditSink) *PaymentRunService {
	return &PaymentRunService{repo: repo, audit: sink}
}

// CashAccounts lists the university's own accounts.
func (s *PaymentRunService) CashAccounts(ctx context.Context) ([]CashAccount, error) {
	return s.repo.ListCashAccounts(ctx)
}

// CreateCashAccount validates and stores a debtor cash account.
func (s *PaymentRunService) CreateCashAccount(ctx context.Context, name, iban, bic, currency string) (CashAccount, error) {
	account, err := NewCashAccount(name, iban, bic, currency)
	if err != nil {
		return CashAccount{}, err
	}
	return s.repo.CreateCashAccount(ctx, account)
}

// CreateRun collects the given approved orders into one SEPA credit-transfer
// file. It is idempotent on the reference: re-issuing the same reference returns
// the already-produced run (and its unchanged file) instead of a second one.
func (s *PaymentRunService) CreateRun(ctx context.Context, reference string, accountID int64,
	executionDate time.Time, orderIDs []int64, by string) (PaymentRun, error) {

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return PaymentRun{}, ErrRunReferenceRequired
	}
	if err := requireActor(by); err != nil {
		return PaymentRun{}, err
	}
	if accountID <= 0 {
		return PaymentRun{}, ErrCashAccountRequired
	}
	if executionDate.IsZero() {
		return PaymentRun{}, ErrExecutionDateRequired
	}
	if len(orderIDs) == 0 {
		return PaymentRun{}, ErrNoOrders
	}

	if existing, ok, err := s.repo.RunByReference(ctx, reference); err != nil {
		return PaymentRun{}, err
	} else if ok {
		return existing, nil
	}

	account, err := s.repo.GetCashAccount(ctx, accountID)
	if err != nil {
		return PaymentRun{}, err
	}

	ids := dedupe(orderIDs)
	orders, err := s.repo.LoadPayableOrders(ctx, ids)
	if err != nil {
		return PaymentRun{}, err
	}
	if len(orders) != len(ids) {
		return PaymentRun{}, fmt.Errorf("kasse: %d of %d orders not found: %w", len(ids)-len(orders), len(ids), ErrNotFound)
	}

	transactions := make([]sepa.Transaction, 0, len(orders))
	items := make([]PaymentRunItem, 0, len(orders))
	for _, o := range orders {
		if o.Status != OrderApproved {
			return PaymentRun{}, fmt.Errorf("kasse: order %d is %s: %w", o.ID, o.Status, ErrOrderNotApproved)
		}
		if !ValidIBAN(o.CreditorIBAN) {
			return PaymentRun{}, fmt.Errorf("kasse: order %d: %w", o.ID, ErrIBANInvalid)
		}
		if o.CreditorBIC != "" && !sepa.ValidBIC(o.CreditorBIC) {
			return PaymentRun{}, fmt.Errorf("kasse: order %d: %w", o.ID, ErrBICInvalid)
		}
		transactions = append(transactions, sepa.Transaction{
			EndToEndID:   o.Reference,
			AmountCents:  o.AmountCents,
			CreditorName: o.CreditorName,
			CreditorIBAN: o.CreditorIBAN,
			CreditorBIC:  o.CreditorBIC,
			Purpose:      o.Purpose,
		})
		items = append(items, PaymentRunItem{OrderID: o.ID, EndToEndID: o.Reference, AmountCents: o.AmountCents})
	}

	ct := sepa.CreditTransfer{
		MessageID:       reference,
		CreationTime:    nowFunc(),
		InitiatingParty: account.Name,
		DebtorName:      account.Name,
		DebtorIBAN:      account.IBAN,
		DebtorBIC:       account.BIC,
		Currency:        account.Currency,
		ExecutionDate:   executionDate,
		Transactions:    transactions,
	}
	fileXML, err := ct.Marshal()
	if err != nil {
		return PaymentRun{}, fmt.Errorf("kasse: build SEPA file: %w", err)
	}

	run := PaymentRun{
		Reference:     reference,
		AccountID:     account.ID,
		ExecutionDate: executionDate,
		MessageID:     reference,
		ItemCount:     len(transactions),
		SumCents:      ct.Sum(),
		FileHash:      hashFile(fileXML),
		CreatedBy:     by,
	}
	saved, err := s.repo.InsertRun(ctx, run, fileXML, items)
	if err != nil {
		return PaymentRun{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "payment_run.create",
		Subject: runSubject(saved.ID),
		After:   fmt.Sprintf("reference=%s items=%d sum_cents=%d hash=%s", saved.Reference, saved.ItemCount, saved.SumCents, saved.FileHash),
	}); err != nil {
		return PaymentRun{}, err
	}
	return saved, nil
}

// Run loads a run.
func (s *PaymentRunService) Run(ctx context.Context, id int64) (PaymentRun, error) {
	return s.repo.GetRun(ctx, id)
}

// RunFile returns the stored bank file of a run.
func (s *PaymentRunService) RunFile(ctx context.Context, id int64) (string, error) {
	return s.repo.RunFile(ctx, id)
}

// hashFile returns the SHA-256 hex digest of the produced file.
func hashFile(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// dedupe removes duplicate ids while preserving order.
func dedupe(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func runSubject(id int64) string { return fmt.Sprintf("payment_run/%d", id) }
