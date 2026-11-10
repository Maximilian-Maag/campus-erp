package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/ledger"
)

func returnsDay(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

type fakeReturnsRepo struct {
	lines    []StatementLine
	err      error
	from, to time.Time
	calls    int
}

func (f *fakeReturnsRepo) ReturnedLines(_ context.Context, from, to time.Time) ([]StatementLine, error) {
	f.calls++
	f.from, f.to = from, to
	return f.lines, f.err
}

// failingAudit is an audit sink that refuses every event (kasse_test.go), used here so the
// export's failure path — nothing is handed over without a record of it — is covered.

// A returning line's counterpart posting takes the direction of the cash movement: a
// debit line (money out) debits the counterpart, a credit line credits it. The line's
// reference is the export's idempotency key, and a line the bank sent without a booking
// date still carries the value date it is effective on.
func TestReturnPostingsExportRendersLines(t *testing.T) {
	repo := &fakeReturnsRepo{lines: []StatementLine{
		{ID: 7, ValueDate: returnsDay(2), BookingDate: returnsDay(3), Credit: false,
			AmountCents: 12500, PostingKey: "009", ClientRef: "ANORD-2026-9", Purpose: "Rückläufer Lastschrift"},
		{ID: 8, ValueDate: returnsDay(1), Credit: true,
			AmountCents: 4999, PostingKey: "059", ClientRef: "ANORD-2026-4", Purpose: "Rückläufer Zahlung"},
	}}
	sink := &fakeAudit{}
	svc := NewReturnPostingsService(repo, sink, DefaultReturnsAccounts())

	exp, err := svc.Export(context.Background(), returnsDay(1), returnsDay(31), "carol")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if repo.calls != 1 || !repo.from.Equal(returnsDay(1)) || !repo.to.Equal(returnsDay(31)) {
		t.Fatalf("repository called with %d, %v..%v", repo.calls, repo.from, repo.to)
	}
	if exp.CashAccount != "1200" || len(exp.Postings) != 2 {
		t.Fatalf("export = %+v", exp)
	}
	first := exp.Postings[0]
	if first.Reference != "RT-000007" || first.Account != "1300" || first.Direction != ledger.Debit {
		t.Fatalf("posting = %+v", first)
	}
	if !first.ValueDate.Equal(returnsDay(2)) || !first.BookingDate.Equal(returnsDay(3)) {
		t.Fatalf("dates = %v / %v", first.ValueDate, first.BookingDate)
	}
	if first.PostingKey != "009" || first.OrderReference != "ANORD-2026-9" || first.Purpose != "Rückläufer Lastschrift" {
		t.Fatalf("posting = %+v", first)
	}
	second := exp.Postings[1]
	if second.Reference != "RT-000008" || second.Direction != ledger.Credit || second.PostingKey != "059" {
		t.Fatalf("posting = %+v", second)
	}
	if !second.BookingDate.IsZero() {
		t.Fatalf("a line without a booking date must not invent one: %v", second.BookingDate)
	}

	got, err := exp.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		"# cash-returns/v1",
		"# cash_account: 1200",
		"RT-000008,2026-10-01,,1300,1200,credit,4999,EUR,059,ANORD-2026-4,Rückläufer Zahlung",
		"RT-000007,2026-10-02,2026-10-03,1300,1200,debit,12500,EUR,009,ANORD-2026-9,Rückläufer Lastschrift",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}

	if len(sink.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(sink.events))
	}
	e := sink.events[0]
	if e.Action != "ledger.return-postings" || e.Actor != "carol" || e.Subject != "ledger/return-postings/2026-10-01..2026-10-31" {
		t.Fatalf("audit event = %+v", e)
	}
	if !strings.Contains(e.After, "postings=2") || !strings.Contains(e.After, "return_account=1300") {
		t.Fatalf("audit detail = %q", e.After)
	}
}

// The screen's read and the export must see the same set, and a period nothing returned
// in is an empty list rather than a refusal.
func TestReturnPostingsReturnsReadsLines(t *testing.T) {
	repo := &fakeReturnsRepo{lines: []StatementLine{{ID: 1, ValueDate: returnsDay(1), AmountCents: 1, PostingKey: "009"}}}
	svc := NewReturnPostingsService(repo, &fakeAudit{}, DefaultReturnsAccounts())

	lines, err := svc.Returns(context.Background(), returnsDay(1), returnsDay(31))
	if err != nil {
		t.Fatalf("Returns: %v", err)
	}
	if len(lines) != 1 || lines[0].ID != 1 {
		t.Fatalf("lines = %+v", lines)
	}
	if repo.calls != 1 || !repo.from.Equal(returnsDay(1)) || !repo.to.Equal(returnsDay(31)) {
		t.Fatalf("repository called with %d, %v..%v", repo.calls, repo.from, repo.to)
	}

	// An inverted period is a refusal on the read too: "nothing came back" and "this is
	// not a period" must not look the same.
	empty := &fakeReturnsRepo{}
	if _, err := NewReturnPostingsService(empty, &fakeAudit{}, DefaultReturnsAccounts()).
		Returns(context.Background(), returnsDay(31), returnsDay(1)); !errors.Is(err, ErrReturnsPeriodInvalid) {
		t.Fatalf("inverted period = %v", err)
	}
	if empty.calls != 0 {
		t.Fatalf("an inverted period must not reach the store (calls = %d)", empty.calls)
	}
}

func TestReturnPostingsExportErrors(t *testing.T) {
	boom := errors.New("boom")
	line := StatementLine{ID: 1, ValueDate: returnsDay(1), AmountCents: 100, PostingKey: "009"}

	cases := []struct {
		name     string
		repo     *fakeReturnsRepo
		sink     AuditSink
		accounts ReturnsAccounts
		from, to time.Time
		actor    string
		want     error
	}{
		{"blank actor", &fakeReturnsRepo{lines: []StatementLine{line}}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "  ", ErrActorRequired},
		{"incomplete accounts", &fakeReturnsRepo{lines: []StatementLine{line}}, &fakeAudit{}, ReturnsAccounts{Cash: "1200"}, returnsDay(1), returnsDay(2), "a", ErrReturnsAccountsIncomplete},
		{"inverted period", &fakeReturnsRepo{lines: []StatementLine{line}}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(31), returnsDay(1), "a", ErrReturnsPeriodInvalid},
		{"no returned lines", &fakeReturnsRepo{}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "a", ErrNoReturnedLines},
		{"repository error", &fakeReturnsRepo{err: boom}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "a", boom},
		{"undated line", &fakeReturnsRepo{lines: []StatementLine{{ID: 4, AmountCents: 100, PostingKey: "009"}}}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "a", ErrReturnLineUndated},
		{"unrenderable line", &fakeReturnsRepo{lines: []StatementLine{{ID: 5, ValueDate: returnsDay(1), AmountCents: 0, PostingKey: "009"}}}, &fakeAudit{}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "a", ledger.ErrAmountNotPositive},
		{"audit error", &fakeReturnsRepo{lines: []StatementLine{line}}, failingAudit{err: boom}, DefaultReturnsAccounts(), returnsDay(1), returnsDay(2), "a", boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := NewReturnPostingsService(c.repo, c.sink, c.accounts)
			_, err := svc.Export(context.Background(), c.from, c.to, c.actor)
			if !errors.Is(err, c.want) {
				t.Fatalf("Export error = %v, want %v", err, c.want)
			}
		})
	}
}

// A refused export must not be recorded as one that happened: the audit event is the
// last thing the service does.
func TestReturnPostingsExportRefusesBeforeAuditing(t *testing.T) {
	sink := &fakeAudit{}
	svc := NewReturnPostingsService(&fakeReturnsRepo{}, sink, DefaultReturnsAccounts())
	if _, err := svc.Export(context.Background(), returnsDay(1), returnsDay(2), "a"); !errors.Is(err, ErrNoReturnedLines) {
		t.Fatalf("Export error = %v", err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("a refused export recorded %d audit events", len(sink.events))
	}
}

func TestReturnsAccounts(t *testing.T) {
	a := DefaultReturnsAccounts()
	if a.Cash == "" || a.Return == "" {
		t.Fatalf("default accounts must both be set: %+v", a)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, blank := range []ReturnsAccounts{
		{Cash: " ", Return: a.Return},
		{Cash: a.Cash, Return: "  "},
	} {
		if err := blank.Validate(); !errors.Is(err, ErrReturnsAccountsIncomplete) {
			t.Fatalf("Validate(%+v) = %v", blank, err)
		}
	}
}

func TestReturnPostingsSubject(t *testing.T) {
	if got := returnPostingsSubject(returnsDay(1), returnsDay(31)); got != "ledger/return-postings/2026-10-01..2026-10-31" {
		t.Fatalf("subject = %q", got)
	}
}
