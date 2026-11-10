package kasse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/ledger"
)

// Return postings to FiBu (docs/30-return-postings.md). A returning bank-statement line
// is money the bank took back — a refused direct debit (posting key 009), a payment-out
// that came back (059). The cash movement is the opposite of the original, so the
// ledger side needs the counterpart posting it reverses; this service collects the
// period's returning lines and renders them as that posting file.
//
// The reference of a return posting is the statement line it came from, so re-exporting
// a period carries the same references and the ledger de-duplicates — the same
// idempotency rule the posting export follows (docs/23).

// Errors of the return-postings export.
var (
	// ErrReturnsPeriodInvalid is returned when the period end precedes its start.
	ErrReturnsPeriodInvalid = errors.New("kasse: the return-postings period end precedes its start")
	// ErrReturnsAccountsIncomplete is returned when the cash or the return account is unset.
	ErrReturnsAccountsIncomplete = errors.New("kasse: the returns accounts must both be set")
	// ErrNoReturnedLines is returned when the period holds no returning statement line.
	ErrNoReturnedLines = errors.New("kasse: no returned statement line in the period")
	// ErrReturnLineUndated is returned when a returning line carries neither a value nor a
	// booking date, so the posting it would produce has no date to take effect on.
	ErrReturnLineUndated = errors.New("kasse: a returned statement line carries no date")
)

// ReturnsAccounts are the accounts a return posting posts against: the cash account that
// received the money back, and the counterpart the returns are cleared to.
type ReturnsAccounts struct {
	Cash   string
	Return string
}

// DefaultReturnsAccounts is the account mapping used unless configuration overrides it.
func DefaultReturnsAccounts() ReturnsAccounts {
	return ReturnsAccounts{Cash: "1200", Return: "1300"}
}

// Validate reports whether both accounts are set.
func (a ReturnsAccounts) Validate() error {
	if strings.TrimSpace(a.Cash) == "" || strings.TrimSpace(a.Return) == "" {
		return ErrReturnsAccountsIncomplete
	}
	return nil
}

// ReturnedLineReader selects the statement lines a return export covers.
type ReturnedLineReader interface {
	ReturnedLines(ctx context.Context, from, to time.Time) ([]StatementLine, error)
}

// ReturnPostingsService renders the return postings of a period and reads the lines
// they are built from, so the screen and the export see the same set.
type ReturnPostingsService struct {
	repo     ReturnedLineReader
	audit    AuditSink
	accounts ReturnsAccounts
}

// NewReturnPostingsService wires the service.
func NewReturnPostingsService(repo ReturnedLineReader, sink AuditSink, accounts ReturnsAccounts) *ReturnPostingsService {
	return &ReturnPostingsService{repo: repo, audit: sink, accounts: accounts}
}

// Returns reads the period's returning statement lines — the reads the screen shows.
// The period is validated here rather than in the store, so an inverted period is a
// refusal and not an empty list that looks like "nothing came back".
func (s *ReturnPostingsService) Returns(ctx context.Context, from, to time.Time) ([]StatementLine, error) {
	if to.Before(from) {
		return nil, ErrReturnsPeriodInvalid
	}
	lines, err := s.repo.ReturnedLines(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("kasse: return postings: %w", err)
	}
	return lines, nil
}

// Export renders the return postings of every returning line in [from, to]. The run is
// recorded on the audit stream like the posting export, and is re-runnable: the
// reference is the statement line, so re-exporting changes nothing.
func (s *ReturnPostingsService) Export(ctx context.Context, from, to time.Time, by string) (ledger.ReturnExport, error) {
	if err := requireActor(by); err != nil {
		return ledger.ReturnExport{}, err
	}
	if err := s.accounts.Validate(); err != nil {
		return ledger.ReturnExport{}, err
	}
	if to.Before(from) {
		return ledger.ReturnExport{}, ErrReturnsPeriodInvalid
	}
	lines, err := s.repo.ReturnedLines(ctx, from, to)
	if err != nil {
		return ledger.ReturnExport{}, fmt.Errorf("kasse: return postings: %w", err)
	}
	if len(lines) == 0 {
		return ledger.ReturnExport{}, ErrNoReturnedLines
	}
	exp := ledger.ReturnExport{
		From:        from,
		To:          to,
		CashAccount: strings.TrimSpace(s.accounts.Cash),
		Postings:    make([]ledger.ReturnPosting, 0, len(lines)),
	}
	for _, line := range lines {
		p, err := s.posting(line)
		if err != nil {
			return ledger.ReturnExport{}, err
		}
		exp.Postings = append(exp.Postings, p)
	}
	// Validate before the audit event: an export that would not render must not be
	// recorded as one that did.
	if err := exp.Validate(); err != nil {
		return ledger.ReturnExport{}, fmt.Errorf("kasse: return postings: %w", err)
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "ledger.return-postings",
		Subject: returnPostingsSubject(from, to),
		After: fmt.Sprintf("postings=%d cash_account=%s return_account=%s",
			len(exp.Postings), exp.CashAccount, strings.TrimSpace(s.accounts.Return)),
	}); err != nil {
		return ledger.ReturnExport{}, err
	}
	return exp, nil
}

// posting maps one returning statement line to the counterpart posting it produces. The
// counterpart takes the direction of the cash movement — money into the account debits
// cash and credits the counterpart — so a credit line credits and a debit line debits.
// The line's own posting key travels with the row, so the ledger can see what the bank
// did, and its client reference ties the return to the order it belonged to.
func (s *ReturnPostingsService) posting(line StatementLine) (ledger.ReturnPosting, error) {
	valueDate := dateOnly(line.ValueDate)
	if valueDate.IsZero() {
		valueDate = dateOnly(line.BookingDate)
	}
	if valueDate.IsZero() {
		return ledger.ReturnPosting{}, fmt.Errorf("%w: line %d", ErrReturnLineUndated, line.ID)
	}
	direction := ledger.Debit
	if line.Credit {
		direction = ledger.Credit
	}
	return ledger.ReturnPosting{
		Reference:      fmt.Sprintf("RT-%06d", line.ID),
		ValueDate:      valueDate,
		BookingDate:    dateOnly(line.BookingDate),
		Account:        strings.TrimSpace(s.accounts.Return),
		Direction:      direction,
		AmountCents:    line.AmountCents,
		PostingKey:     line.PostingKey,
		OrderReference: line.ClientRef,
		Purpose:        line.Purpose,
	}, nil
}

func returnPostingsSubject(from, to time.Time) string {
	return fmt.Sprintf("ledger/return-postings/%s..%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
}
