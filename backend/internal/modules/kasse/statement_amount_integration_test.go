package kasse_test

import (
	"context"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestStatementLineMatchesOnAmountAndDirection covers #170: a reference alone
// does not make a line the settlement of a paid order. A line that runs the
// other way, or carries a different amount, must stay unmatched so the order is
// not reported as settled while it is short — and the shortfall must be visible.
func TestStatementLineMatchesOnAmountAndDirection(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	exact := newApprovedOrder(t, orderSvc, 100000)
	short := newApprovedOrder(t, orderSvc, 100000)
	incoming := newApprovedOrder(t, orderSvc, 100000)

	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{exact.ID, short.ID, incoming.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	content := mt940File("STMT-"+uniqueSuffix(t), []mtLine{
		{clientRef: exact.Reference, credit: false, cents: 100000, key: "051"},   // exact settlement
		{clientRef: short.Reference, credit: false, cents: 99000, key: "051"},    // 1,000 short
		{clientRef: incoming.Reference, credit: true, cents: 100000, key: "051"}, // wrong direction
	})

	stmts, err := stmtSvc.Import(ctx, account.ID, content, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 || len(stmts[0].Lines) != 3 {
		t.Fatalf("statements = %+v, want one statement with three lines", stmts)
	}
	lines := stmts[0].Lines

	if lines[0].Status != kasse.StatusMatched || lines[0].OrderID == nil || *lines[0].OrderID != exact.ID {
		t.Fatalf("exact line = %+v, want matched to order %d", lines[0], exact.ID)
	}
	if lines[0].UnclearedCents != 0 {
		t.Fatalf("exact line uncleared = %d, want 0", lines[0].UnclearedCents)
	}

	if lines[1].Status != kasse.StatusUnmatched || lines[1].OrderID != nil {
		t.Fatalf("short line = %+v, want unmatched (a short payment is not a settlement)", lines[1])
	}
	if lines[1].UnclearedCents != 1000 {
		t.Fatalf("short line uncleared = %d, want 1000", lines[1].UnclearedCents)
	}

	if lines[2].Status != kasse.StatusUnmatched || lines[2].OrderID != nil {
		t.Fatalf("incoming line = %+v, want unmatched (an incoming credit cannot settle an outbound order)", lines[2])
	}
}
