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

// mt940File builds a minimal MT940 statement from lines. Each line is given as
// (client reference, credit, amount in cents, posting key).
func mt940File(ref string, lines []struct {
	clientRef string
	credit    bool
	cents     int64
	key       string
}) string {
	var b strings.Builder
	fmt.Fprintf(&b, ":20:%s\n", ref)
	b.WriteString(":25:DE02120300000000202051\n")
	b.WriteString(":28C:00001/001\n")
	b.WriteString(":60F:C261001EUR10000,00\n")
	for i, l := range lines {
		dc := "D"
		if l.credit {
			dc = "C"
		}
		fmt.Fprintf(&b, ":61:26100%d100%d%s%d,%02dN%s%s//BANK%d\n",
			(i+1)%10, (i+1)%10, dc, l.cents/100, l.cents%100, l.key, l.clientRef, i+1)
		fmt.Fprintf(&b, ":86:Buchung %d\n", i+1)
	}
	b.WriteString(":62F:C261031EUR10000,00\n")
	b.WriteString("-\n")
	return b.String()
}

type mtLine = struct {
	clientRef string
	credit    bool
	cents     int64
	key       string
}

func TestStatementImportReconciles(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	a := newApprovedOrder(t, orderSvc, 125000)
	b := newApprovedOrder(t, orderSvc, 4999)

	account, err := runSvc.CreateBankAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateBankAccount: %v", err)
	}
	// Pay both orders so they count as matched.
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{a.ID, b.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	content := mt940File("STMT-"+uniqueSuffix(t), []mtLine{
		{clientRef: a.Reference, credit: false, cents: 125000, key: "051"}, // matched
		{clientRef: b.Reference, credit: true, cents: 4999, key: "059"},   // returned (059)
		{clientRef: "FREMDBUCHUNG", credit: true, cents: 5000, key: "051"}, // unmatched
	})

	stmts, err := stmtSvc.Import(ctx, account.ID, content, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want 1", len(stmts))
	}
	st := stmts[0]
	if st.LineCount != 3 || len(st.Lines) != 3 {
		t.Fatalf("lines = %d/%d, want 3", st.LineCount, len(st.Lines))
	}
	if st.Lines[0].Status != kasse.StatusMatched || st.Lines[0].OrderID == nil || *st.Lines[0].OrderID != a.ID {
		t.Fatalf("line 0 = %+v, want matched to order %d", st.Lines[0], a.ID)
	}
	if st.Lines[1].Status != kasse.StatusReturned {
		t.Fatalf("line 1 status = %q, want returned", st.Lines[1].Status)
	}
	if st.Lines[2].Status != kasse.StatusUnmatched || st.Lines[2].OrderID != nil {
		t.Fatalf("line 2 = %+v, want unmatched", st.Lines[2])
	}

	// Idempotency: re-importing the same file returns the same statement.
	again, err := stmtSvc.Import(ctx, account.ID, content, "carol")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(again) != 1 || again[0].ID != st.ID {
		t.Fatalf("re-import produced statement %+v, want id %d", again, st.ID)
	}

	// Loaded back with lines.
	loaded, err := stmtSvc.Statement(ctx, st.ID)
	if err != nil {
		t.Fatalf("Statement: %v", err)
	}
	if len(loaded.Lines) != 3 || loaded.OpeningBalanceCents != 1000000 {
		t.Fatalf("loaded = %+v", loaded)
	}

	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestStatementImportValidation(t *testing.T) {
	ctx := context.Background()
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	if _, err := stmtSvc.Import(ctx, 1, ":20:A\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n", ""); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	if _, err := stmtSvc.Import(ctx, 0, "x", "carol"); !errors.Is(err, kasse.ErrBankAccountRequired) {
		t.Fatalf("no account: %v", err)
	}
	if _, err := stmtSvc.Import(ctx, 1, "   ", "carol"); !errors.Is(err, kasse.ErrStatementEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := stmtSvc.Import(ctx, 1, ":20:A\n:61:garbage\n", "carol"); !errors.Is(err, kasse.ErrStatementInvalid) {
		t.Fatalf("invalid: %v", err)
	}
	if _, err := stmtSvc.Statement(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrStatementNotFound) {
		t.Fatalf("not found: %v", err)
	}
}
