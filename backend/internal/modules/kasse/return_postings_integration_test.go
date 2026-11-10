package kasse_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// returnsDay is the value date the return-postings integration test's statement carries.
// It is a fixed day, away from the dates the other integration tests use, because the
// database persists between runs: the period is stated, not derived.
func returnsDay(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }

// mt940WithReturns builds a statement carrying one returning line per (order reference,
// amount, credit, posting key) — the lines the bank took back.
func mt940WithReturns(ref, iban string, lines []struct {
	orderRef string
	cents    int64
	credit   bool
	key      string
}) string {
	var b strings.Builder
	fmt.Fprintf(&b, ":20:%s\n", ref)
	fmt.Fprintf(&b, ":25:%s\n", iban)
	b.WriteString(":28C:00001/001\n")
	b.WriteString(":60F:C261120EUR10000,00\n")
	for i, l := range lines {
		dc := "D"
		if l.credit {
			dc = "C"
		}
		fmt.Fprintf(&b, ":61:2611201120%s%d,%02dN%s%s//BANK%d\n", dc, l.cents/100, l.cents%100, l.key, l.orderRef, i+1)
		fmt.Fprintf(&b, ":86:Rückläufer Buchung %d\n", i+1)
	}
	b.WriteString(":62F:C261120EUR10000,00\n")
	b.WriteString("-\n")
	return b.String()
}

// TestReturnPostingsIntegration covers the whole path against a real database: returning
// statement lines are selected by their value date, the export renders one counterpart
// posting per line with the direction of the cash movement and the bank's posting key,
// and a period nothing came back in is refused rather than handed over empty.
func TestReturnPostingsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(pool), audit.NewStore(pool))

	// Two paid orders: one the bank returns as a refused debit (009), one as a returned
	// payment-out (059), so both directions are exercised.
	debitOrder := newApprovedOrder(t, orderSvc, 125000)
	creditOrder := newApprovedOrder(t, orderSvc, 4999)

	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		returnsDay(18), []int64{debitOrder.ID, creditOrder.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	content := mt940WithReturns("STMT-RETURNS-"+uniqueSuffix(t), "DE02120300000000202051", []struct {
		orderRef string
		cents    int64
		credit   bool
		key      string
	}{
		{orderRef: debitOrder.Reference, cents: 125000, credit: false, key: "009"},
		{orderRef: creditOrder.Reference, cents: 4999, credit: true, key: "059"},
	})
	if _, err := stmtSvc.Import(ctx, account.ID, content, "carol"); err != nil {
		t.Fatalf("Import: %v", err)
	}

	svc := kasse.NewReturnPostingsService(kasse.NewStatementRepo(pool), audit.NewStore(pool), kasse.DefaultReturnsAccounts())

	// The read the screen lists and the export must cover the same set.
	lines, err := svc.Returns(ctx, returnsDay(20), returnsDay(20))
	if err != nil {
		t.Fatalf("Returns: %v", err)
	}
	byRef := map[string]kasse.StatementLine{}
	for _, l := range lines {
		byRef[l.ClientRef] = l
	}
	debitLine, ok := byRef[debitOrder.Reference]
	if !ok {
		t.Fatalf("the returned debit line is not among %d lines for %s", len(lines), returnsDay(20).Format("2006-01-02"))
	}
	creditLine, ok := byRef[creditOrder.Reference]
	if !ok {
		t.Fatalf("the returned credit line is not among %d lines", len(lines))
	}
	if debitLine.Status != kasse.StatusReturned || !debitLine.Return || debitLine.PostingKey != "009" {
		t.Fatalf("debit line = %+v", debitLine)
	}
	if creditLine.Status != kasse.StatusReturned || !creditLine.Return || creditLine.PostingKey != "059" {
		t.Fatalf("credit line = %+v", creditLine)
	}

	exp, err := svc.Export(ctx, returnsDay(20), returnsDay(20), "carol")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(exp.Postings) != len(lines) {
		t.Fatalf("export carries %d postings for %d returning lines", len(exp.Postings), len(lines))
	}
	postings := map[string]bool{}
	dirs := map[string]string{}
	for _, p := range exp.Postings {
		postings[p.Reference] = true
		dirs[p.OrderReference] = string(p.Direction)
		if p.Account != "1300" {
			t.Fatalf("posting %s posts against %q", p.Reference, p.Account)
		}
		if !p.ValueDate.Equal(returnsDay(20)) {
			t.Fatalf("posting %s takes effect on %v", p.Reference, p.ValueDate)
		}
	}
	if !postings[fmt.Sprintf("RT-%06d", debitLine.ID)] || !postings[fmt.Sprintf("RT-%06d", creditLine.ID)] {
		t.Fatalf("the returning lines are missing from the export: %+v", exp.Postings)
	}
	// The counterpart takes the direction of the cash movement.
	if dirs[debitOrder.Reference] != "debit" || dirs[creditOrder.Reference] != "credit" {
		t.Fatalf("directions = %+v", dirs)
	}

	got, err := exp.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		"# cash-returns/v1",
		"# period: 2026-11-20..2026-11-20",
		"# cash_account: 1200",
		fmt.Sprintf("RT-%06d,2026-11-20,2026-11-20,1300,1200,debit,125000,EUR,009,%s", debitLine.ID, debitOrder.Reference),
		fmt.Sprintf("RT-%06d,2026-11-20,2026-11-20,1300,1200,credit,4999,EUR,059,%s", creditLine.ID, creditOrder.Reference),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}

	// The reference is the statement line, so a second export of the same period is
	// identical: re-exporting does not produce a second posting for the same return.
	again, err := svc.Export(ctx, returnsDay(20), returnsDay(20), "carol")
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	second, err := again.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if second != got {
		t.Fatalf("a re-export changed the file:\n%s\n---\n%s", got, second)
	}

	// A period nothing came back in is a refusal, not an empty file handed to the
	// ledger side.
	if _, err := svc.Export(ctx, returnsDay(1), returnsDay(2), "carol"); !errors.Is(err, kasse.ErrNoReturnedLines) {
		t.Fatalf("an empty period = %v", err)
	}
}
