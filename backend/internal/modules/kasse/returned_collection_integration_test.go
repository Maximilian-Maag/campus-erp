package kasse_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// mt940ReturningDebit builds a statement carrying one returning direct debit: the bank took
// a collected amount back, so the line is a debit with the direct-debit return posting key
// and names the original collection in its end-to-end reference — which is the mandate
// reference, because that is what a collection run from receivables sends.
func mt940ReturningDebit(ref, iban, statementRef, valueDate string, cents int64, mandateRef string) string {
	var b strings.Builder
	fmt.Fprintf(&b, ":20:%s\n", ref)
	fmt.Fprintf(&b, ":25:%s\n", iban)
	b.WriteString(":28C:00001/001\n")
	b.WriteString(":60F:C261122EUR10000,00\n")
	fmt.Fprintf(&b, ":61:%s1122D%d,%02dN009%s//BANK1\n", valueDate, cents/100, cents%100, mandateRef)
	fmt.Fprintf(&b, ":86:EREF+%s SVWZ+Ruecklaeufer Zahlungspflichtiger\n", statementRef)
	b.WriteString(":62F:C261122EUR10000,00\n")
	b.WriteString("-\n")
	return b.String()
}

// TestReturnedCollectionReleasesTheReceivable covers the whole path of a refused direct debit
// against a real database: a receivable is collected by a run, the bank returns the amount, and
// the import has to put the item back — an item left 'collected' is selected neither by dunning
// nor by the next run, so the debt would be chased by nothing while the books said it was paid.
// The same import has to make the return visible to the ledger export, which cannot match it to
// a payment order: a collection has no payment order to match.
func TestReturnedCollectionReleasesTheReceivable(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)

	cash := kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store)
	iban := uniqueIBAN(t)
	account, err := cash.CreateCashAccount(ctx, "Universitaet Musterstadt", iban, "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}

	mandateRef := "MAND-" + uniqueSuffix(t)
	mandates := kasse.NewMandateService(kasse.NewMandateRepo(pool), store)
	m, err := kasse.NewMandate(mandateRef, "Max Mustermann", "DE89370400440532013000", "COBADEFFXXX",
		time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC), kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if _, err := mandates.CreateMandate(ctx, m, "clerk"); err != nil {
		t.Fatalf("CreateMandate: %v", err)
	}

	recv := kasse.NewReceivableService(kasse.NewReceivableRepo(pool), store)
	dueDate := returnsDay(21)
	item, err := recv.Post(ctx, mandateRef, 4250, "EUR", dueDate, "Semesterbeitrag", "test", "clerk")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	// The run collects the item and produces its file: the receivable is collected and points
	// at the run, which is the state the return has to undo.
	repo := kasse.NewReceivableRepo(pool)
	collection := kasse.NewCollectionRunService(kasse.NewCollectionRunRepo(pool), kasse.NewPaymentRunRepo(pool),
		kasse.NewMandateRepo(pool), repo, store)
	collected, err := collection.CreateCollectionRunFromReceivables(ctx, "COL-"+uniqueSuffix(t), account.ID,
		"DE98ZZZ09999999999", returnsDay(22), kasse.SequenceRecurring, "CORE", dueDate, "clerk")
	if err != nil {
		t.Fatalf("CreateCollectionRunFromReceivables: %v", err)
	}
	if got, err := repo.GetReceivable(ctx, item.ID); err != nil {
		t.Fatalf("GetReceivable: %v", err)
	} else if got.State != kasse.ReceivableCollected || got.CollectedRunID == nil || *got.CollectedRunID != collected.ID {
		t.Fatalf("receivable before the return = %+v, want collected by run %d", got, collected.ID)
	}

	// The debtor's bank returns the collection; the statement names the mandate as the original
	// end-to-end reference.
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(pool), store)
	content := mt940ReturningDebit("STMT-"+uniqueSuffix(t), iban, mandateRef, "261122", 4250, mandateRef)
	stmts, err := stmtSvc.Import(ctx, account.ID, content, "clerk")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 || len(stmts[0].Lines) != 1 {
		t.Fatalf("statements = %+v", stmts)
	}
	line := stmts[0].Lines[0]
	if !line.Return || line.Status != kasse.StatusReturned {
		t.Fatalf("line = %+v, want a returned line", line)
	}
	if line.OrderID != nil {
		t.Fatalf("a refused direct debit matched a payment order: %+v", line)
	}

	// The debt is owed again: open, and no longer pointing at the run that collected it.
	released, err := repo.GetReceivable(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetReceivable: %v", err)
	}
	if released.State != kasse.ReceivableOpen || released.CollectedRunID != nil {
		t.Fatalf("receivable after the return = %+v, want open with no run", released)
	}

	// Both selectors the debt lives by see it again: the dunning run's and the next collection's.
	due, err := repo.OpenReceivablesDueBy(ctx, returnsDay(22))
	if err != nil {
		t.Fatalf("OpenReceivablesDueBy: %v", err)
	}
	if !containsReceivable(due, item.ID) {
		t.Fatalf("the released item is not dunnable: %+v", due)
	}
	again, err := repo.OpenReceivablesForCollection(ctx, returnsDay(22), kasse.SequenceRecurring)
	if err != nil {
		t.Fatalf("OpenReceivablesForCollection: %v", err)
	}
	if !containsReceivable(again, item.ID) {
		t.Fatalf("the released item is not collectable again: %+v", again)
	}

	// And the return reaches the ledger export, which is the point of the export: a refused
	// direct debit is a posting key 009 posting even though no payment order exists.
	svc := kasse.NewReturnPostingsService(kasse.NewStatementRepo(pool), store, kasse.DefaultReturnsAccounts())
	exp, err := svc.Export(ctx, returnsDay(22), returnsDay(22), "clerk")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := fmt.Sprintf("RT-%06d", line.ID)
	var found bool
	for _, p := range exp.Postings {
		if p.Reference != want {
			continue
		}
		found = true
		if p.PostingKey != "009" || p.AmountCents != 4250 || p.OrderReference != mandateRef ||
			p.Direction != "debit" {
			t.Fatalf("posting = %+v", p)
		}
	}
	if !found {
		t.Fatalf("the refused direct debit is missing from the export: %+v", exp.Postings)
	}
}

func containsReceivable(recs []kasse.Receivable, id int64) bool {
	for _, r := range recs {
		if r.ID == id {
			return true
		}
	}
	return false
}
