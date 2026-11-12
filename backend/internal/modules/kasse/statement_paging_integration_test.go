package kasse_test

import (
	"context"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestStatementListPages drives the statement list the way the screen does — one page at a time,
// advancing the offset — and checks the three properties a paged, filtered list must have: a page
// holds no more than was asked for, walking the pages yields exactly the account's whole list in
// order (nothing repeated, nothing skipped), and the filter holds across the pages rather than only
// on the first one.
//
// The rows are created here rather than assumed, because the list is narrowed to one account: the
// test owns every row it reads, so the expected answer is exact.
func TestStatementListPages(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo := kasse.NewStatementRepo(pool)
	stmtSvc := kasse.NewStatementService(repo, audit.NewStore(pool))
	runSvc, _ := newRunService(t)

	mark := uniqueSuffix(t)
	// The IBANs come from the package's helper: a hand-written one looks right and fails the
	// checksum, which the account service checks.
	account, err := runSvc.CreateCashAccount(ctx, "Uni-Seiten-"+mark, uniqueIBAN(t), "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	other, err := runSvc.CreateCashAccount(ctx, "Uni-Seiten-Andere-"+mark, uniqueIBAN(t), "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	file, err := repo.InsertBankFile(ctx, kasse.BankFile{
		AccountID: account.ID, FileName: "seiten-" + mark, FileHash: "hash-seiten-" + mark, ImportedBy: "carol",
	})
	if err != nil {
		t.Fatalf("InsertBankFile: %v", err)
	}
	otherFile, err := repo.InsertBankFile(ctx, kasse.BankFile{
		AccountID: other.ID, FileName: "seiten-andere-" + mark, FileHash: "hash-andere-" + mark, ImportedBy: "carol",
	})
	if err != nil {
		t.Fatalf("InsertBankFile: %v", err)
	}

	// Five statements on the account under test, one on another: a page boundary that crossed the
	// filter would show up as a row of the wrong account rather than as a missing one.
	rows := make([]kasse.Statement, 0, 5)
	for i := 1; i <= 5; i++ {
		rows = append(rows, kasse.Statement{
			AccountID: account.ID, Reference: "STMT-" + mark + "-" + string(rune('a'+i-1)),
			Number: i, Page: 1, Currency: "EUR", LineCount: 0, ImportedBy: "carol",
		})
	}
	if _, err := repo.InsertStatements(ctx, file.ID, rows); err != nil {
		t.Fatalf("InsertStatements: %v", err)
	}
	if _, err := repo.InsertStatements(ctx, otherFile.ID, []kasse.Statement{{
		AccountID: other.ID, Reference: "STMT-" + mark + "-other", Number: 1, Page: 1,
		Currency: "EUR", LineCount: 0, ImportedBy: "carol",
	}}); err != nil {
		t.Fatalf("InsertStatements (other): %v", err)
	}

	whole, err := stmtSvc.Statements(ctx, account.ID, 200, 0)
	if err != nil {
		t.Fatalf("Statements: %v", err)
	}
	if len(whole) != 5 {
		t.Fatalf("the account lists %d statements, want the 5 just created", len(whole))
	}
	for _, s := range whole {
		if s.AccountID != account.ID {
			t.Fatalf("statement %d belongs to account %d, not %d", s.ID, s.AccountID, account.ID)
		}
	}

	const pageSize = 2
	var walked []kasse.StatementSummary
	for offset := 0; offset < len(whole); offset += pageSize {
		page, err := stmtSvc.Statements(ctx, account.ID, pageSize, offset)
		if err != nil {
			t.Fatalf("Statements(offset=%d): %v", offset, err)
		}
		if len(page) > pageSize {
			t.Fatalf("page at offset %d holds %d rows, want at most %d", offset, len(page), pageSize)
		}
		for _, s := range page {
			if s.AccountID != account.ID {
				t.Fatalf("page at offset %d returned a statement of account %d", offset, s.AccountID)
			}
		}
		walked = append(walked, page...)
	}
	if len(walked) != len(whole) {
		t.Fatalf("walking the pages yielded %d statements, want %d", len(walked), len(whole))
	}
	for i := range whole {
		if walked[i].ID != whole[i].ID {
			t.Fatalf("page walk diverges at %d: got %d, want %d — the order is not total", i, walked[i].ID, whole[i].ID)
		}
	}

	past, err := stmtSvc.Statements(ctx, account.ID, pageSize, len(whole)+pageSize)
	if err != nil {
		t.Fatalf("Statements past the end: %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows, want none", len(past))
	}
}
