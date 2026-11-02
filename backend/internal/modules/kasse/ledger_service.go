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

// Errors of the posting export.
var (
	ErrLedgerPeriodInvalid      = errors.New("kasse: ledger period end precedes its start")
	ErrLedgerAccountsIncomplete = errors.New("kasse: ledger accounts must all be set")
	ErrNoBookings               = errors.New("kasse: no confirmed bookings in the period")
	ErrLedgerUnbalanced         = errors.New("kasse: the ledger export does not balance against its bookings")
)

// LedgerAccounts are the accounts a posting export posts against.
type LedgerAccounts struct {
	Cash    string
	Income  string
	Expense string
}

// DefaultLedgerAccounts is the account mapping used unless configuration overrides it.
func DefaultLedgerAccounts() LedgerAccounts {
	return LedgerAccounts{Cash: "1200", Income: "8400", Expense: "4400"}
}

// Validate reports whether every account is set.
func (a LedgerAccounts) Validate() error {
	if strings.TrimSpace(a.Cash) == "" || strings.TrimSpace(a.Income) == "" || strings.TrimSpace(a.Expense) == "" {
		return ErrLedgerAccountsIncomplete
	}
	return nil
}

// LedgerExportService collects a period's confirmed bookings and renders the posting
// export that hands them to the ledger side (docs/23).
type LedgerExportService struct {
	repo     LedgerRepository
	audit    AuditSink
	accounts LedgerAccounts
}

// NewLedgerExportService wires the export service.
func NewLedgerExportService(repo LedgerRepository, sink AuditSink, accounts LedgerAccounts) *LedgerExportService {
	return &LedgerExportService{repo: repo, audit: sink, accounts: accounts}
}

// Export renders the postings for every booking made on a day in [from, to]. The run
// is recorded on the audit stream, like the ETL runs, and is re-runnable: the
// references are the idempotency key, so re-exporting a period changes nothing.
func (s *LedgerExportService) Export(ctx context.Context, from, to time.Time, by string) (ledger.Export, error) {
	if err := requireActor(by); err != nil {
		return ledger.Export{}, err
	}
	if err := s.accounts.Validate(); err != nil {
		return ledger.Export{}, err
	}
	if to.Before(from) {
		return ledger.Export{}, ErrLedgerPeriodInvalid
	}
	rows, err := s.repo.ListBookingsForExport(ctx, from, to)
	if err != nil {
		return ledger.Export{}, fmt.Errorf("kasse: ledger export: %w", err)
	}
	if len(rows) == 0 {
		return ledger.Export{}, ErrNoBookings
	}
	exp := ledger.Export{
		From:        from,
		To:          to,
		CashAccount: strings.TrimSpace(s.accounts.Cash),
		Postings:    make([]ledger.Posting, 0, len(rows)),
	}
	for _, b := range rows {
		p, err := s.posting(b)
		if err != nil {
			return ledger.Export{}, err
		}
		exp.Postings = append(exp.Postings, p)
	}
	// Tie-back: every collected booking must have become exactly one posting. The
	// result goes on the audit stream, so an export that does not account for its
	// bookings is visible rather than silent.
	refs := make([]string, 0, len(rows))
	for _, b := range rows {
		refs = append(refs, b.Reference)
	}
	rec := ledger.Reconcile(refs, exp.Postings)
	if !rec.Balanced() {
		// Unreachable while postings are built one-for-one from the rows, which is the
		// point: it is an invariant, and an export that does not account for its
		// bookings must never leave the service.
		return ledger.Export{}, fmt.Errorf("%w: missing=%v extra=%v", ErrLedgerUnbalanced, rec.Missing, rec.Extra)
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "ledger.export",
		Subject: ledgerPeriodSubject(from, to),
		After: fmt.Sprintf("postings=%d cash_account=%s matched=%d missing=%d extra=%d balanced=%t",
			len(exp.Postings), exp.CashAccount, rec.Matched, len(rec.Missing), len(rec.Extra), rec.Balanced()),
	}); err != nil {
		return ledger.Export{}, err
	}
	return exp, nil
}

// posting maps one booking to a posting against the cash account. An income credits
// the counterpart account and an expense debits it, so the direction on the row is
// the direction of the counterpart posting.
func (s *LedgerExportService) posting(b LedgerBooking) (ledger.Posting, error) {
	var account string
	var direction ledger.Direction
	if b.Kind == Income {
		account, direction = s.accounts.Income, ledger.Credit
	} else if b.Kind == Expense {
		account, direction = s.accounts.Expense, ledger.Debit
	} else {
		return ledger.Posting{}, ErrUnknownKind
	}
	return ledger.Posting{
		Reference:   b.Reference,
		BookingDate: b.BusinessDate,
		Account:     strings.TrimSpace(account),
		Direction:   direction,
		AmountCents: b.AmountCents,
		Purpose:     b.Purpose,
	}, nil
}

func ledgerPeriodSubject(from, to time.Time) string {
	return fmt.Sprintf("ledger/%s..%s", from.Format("2006-01-02"), to.Format("2006-01-02"))
}
