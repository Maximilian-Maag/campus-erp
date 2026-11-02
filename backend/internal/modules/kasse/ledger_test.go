package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/ledger"
)

func ledgerDay(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

type fakeLedgerRepo struct {
	rows     []LedgerBooking
	err      error
	from, to time.Time
	calls    int
}

func (f *fakeLedgerRepo) ListBookingsForExport(_ context.Context, from, to time.Time) ([]LedgerBooking, error) {
	f.calls++
	f.from, f.to = from, to
	return f.rows, f.err
}

type fakeLedgerAudit struct {
	events []audit.Event
	err    error
}

func (f *fakeLedgerAudit) Append(_ context.Context, e audit.Event) (audit.Event, error) {
	if f.err != nil {
		return audit.Event{}, f.err
	}
	f.events = append(f.events, e)
	return e, nil
}

func TestLedgerExportRendersPostings(t *testing.T) {
	repo := &fakeLedgerRepo{rows: []LedgerBooking{
		{Reference: "BK-000001", BusinessDate: ledgerDay(1), Kind: Income, AmountCents: 125000, Purpose: "Stromabschlag"},
		{Reference: "BK-000002", BusinessDate: ledgerDay(2), Kind: Expense, AmountCents: 4999, Purpose: "Fachliteratur"},
	}}
	auditSink := &fakeLedgerAudit{}
	svc := NewLedgerExportService(repo, auditSink, DefaultLedgerAccounts())

	exp, err := svc.Export(context.Background(), ledgerDay(1), ledgerDay(31), "  alice ")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if repo.calls != 1 || !repo.from.Equal(ledgerDay(1)) || !repo.to.Equal(ledgerDay(31)) {
		t.Fatalf("repository called with %d, %v..%v", repo.calls, repo.from, repo.to)
	}
	if exp.CashAccount != "1200" || len(exp.Postings) != 2 {
		t.Fatalf("export = %+v", exp)
	}
	// income credits the counterpart account, expense debits it
	if p := exp.Postings[0]; p.Account != "8400" || p.Direction != ledger.Credit {
		t.Fatalf("income posting = %+v", p)
	}
	if p := exp.Postings[1]; p.Account != "4400" || p.Direction != ledger.Debit {
		t.Fatalf("expense posting = %+v", p)
	}

	got, err := exp.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		"# cash-postings/v1",
		"BK-000001,2026-10-01,8400,1200,credit,125000,EUR,Stromabschlag",
		"BK-000002,2026-10-02,4400,1200,debit,4999,EUR,Fachliteratur",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}
	if len(auditSink.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(auditSink.events))
	}
	e := auditSink.events[0]
	if e.Action != "ledger.export" || e.Actor != "  alice " || e.Subject != "ledger/2026-10-01..2026-10-31" {
		t.Fatalf("audit event = %+v", e)
	}
	if !strings.Contains(e.After, "postings=2") {
		t.Fatalf("audit detail = %q", e.After)
	}
}

func TestLedgerExportIsDeterministic(t *testing.T) {
	exp, err := NewLedgerExportService(&fakeLedgerRepo{rows: []LedgerBooking{
		{Reference: "BK-000002", BusinessDate: ledgerDay(2), Kind: Expense, AmountCents: 100, Purpose: "b"},
		{Reference: "BK-000001", BusinessDate: ledgerDay(1), Kind: Income, AmountCents: 200, Purpose: "a"},
	}}, &fakeLedgerAudit{}, DefaultLedgerAccounts()).Export(context.Background(), ledgerDay(1), ledgerDay(31), "a")
	if err != nil {
		t.Fatal(err)
	}
	first, err := exp.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	second, err := exp.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("an export must render identically twice")
	}
	if strings.Index(first, "BK-000001") > strings.Index(first, "BK-000002") {
		t.Fatalf("postings must be ordered by booking date:\n%s", first)
	}
}

func TestLedgerExportErrors(t *testing.T) {
	boom := errors.New("boom")
	booking := LedgerBooking{Reference: "BK-1", BusinessDate: ledgerDay(1), Kind: Income, AmountCents: 100}

	cases := []struct {
		name     string
		repo     *fakeLedgerRepo
		audit    *fakeLedgerAudit
		accounts LedgerAccounts
		from, to time.Time
		actor    string
		want     error
	}{
		{"blank actor", &fakeLedgerRepo{rows: []LedgerBooking{booking}}, &fakeLedgerAudit{}, DefaultLedgerAccounts(), ledgerDay(1), ledgerDay(2), "  ", ErrActorRequired},
		{"incomplete accounts", &fakeLedgerRepo{rows: []LedgerBooking{booking}}, &fakeLedgerAudit{}, LedgerAccounts{Cash: "1200"}, ledgerDay(1), ledgerDay(2), "a", ErrLedgerAccountsIncomplete},
		{"inverted period", &fakeLedgerRepo{rows: []LedgerBooking{booking}}, &fakeLedgerAudit{}, DefaultLedgerAccounts(), ledgerDay(31), ledgerDay(1), "a", ErrLedgerPeriodInvalid},
		{"no bookings", &fakeLedgerRepo{}, &fakeLedgerAudit{}, DefaultLedgerAccounts(), ledgerDay(1), ledgerDay(2), "a", ErrNoBookings},
		{"repository error", &fakeLedgerRepo{err: boom}, &fakeLedgerAudit{}, DefaultLedgerAccounts(), ledgerDay(1), ledgerDay(2), "a", boom},
		{"unknown kind", &fakeLedgerRepo{rows: []LedgerBooking{{Reference: "BK-1", BusinessDate: ledgerDay(1), Kind: "gift", AmountCents: 1}}}, &fakeLedgerAudit{}, DefaultLedgerAccounts(), ledgerDay(1), ledgerDay(2), "a", ErrUnknownKind},
		{"audit error", &fakeLedgerRepo{rows: []LedgerBooking{booking}}, &fakeLedgerAudit{err: boom}, DefaultLedgerAccounts(), ledgerDay(1), ledgerDay(2), "a", boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewLedgerExportService(c.repo, c.audit, c.accounts)
			if _, err := svc.Export(context.Background(), c.from, c.to, c.actor); !errors.Is(err, c.want) {
				t.Fatalf("Export error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestLedgerAccounts(t *testing.T) {
	a := DefaultLedgerAccounts()
	if a.Cash == "" || a.Income == "" || a.Expense == "" {
		t.Fatalf("default accounts must all be set: %+v", a)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, blank := range []LedgerAccounts{
		{Cash: " ", Income: a.Income, Expense: a.Expense},
		{Cash: a.Cash, Income: "", Expense: a.Expense},
		{Cash: a.Cash, Income: a.Income, Expense: "  "},
	} {
		if err := blank.Validate(); !errors.Is(err, ErrLedgerAccountsIncomplete) {
			t.Fatalf("Validate(%+v) = %v", blank, err)
		}
	}
}

func TestLedgerPeriodSubject(t *testing.T) {
	if got := ledgerPeriodSubject(ledgerDay(1), ledgerDay(31)); got != "ledger/2026-10-01..2026-10-31" {
		t.Fatalf("subject = %q", got)
	}
}
