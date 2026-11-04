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

	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	// Pay both orders so they count as matched.
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{a.ID, b.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	content := mt940File("STMT-"+uniqueSuffix(t), []mtLine{
		{clientRef: a.Reference, credit: false, cents: 125000, key: "051"}, // matched
		{clientRef: b.Reference, credit: true, cents: 4999, key: "059"},    // returned (059)
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

	// The statement belongs to an imported file.
	if st.BankFileID == 0 {
		t.Fatal("BankFileID must be set")
	}
	file, err := stmtSvc.BankFile(ctx, st.BankFileID)
	if err != nil {
		t.Fatalf("BankFile: %v", err)
	}
	if file.FileHash == "" || file.AccountID != account.ID {
		t.Fatalf("bank file = %+v", file)
	}

	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestStatementImportMultipleStatementsInOneFile covers "one file, many
// statements": both statements share one bank_file.
func TestStatementImportMultipleStatementsInOneFile(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	_ = orderSvc

	two := mt940File("STMT-A-"+uniqueSuffix(t), []mtLine{{clientRef: "X", credit: true, cents: 100, key: "051"}}) +
		strings.Replace(mt940File("STMT-B-"+uniqueSuffix(t), []mtLine{{clientRef: "Y", credit: false, cents: 200, key: "051"}}),
			":28C:00001/001", ":28C:00002/001", 1)
	stmts, err := stmtSvc.Import(ctx, account.ID, two, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("statements = %d, want 2", len(stmts))
	}
	if stmts[0].BankFileID != stmts[1].BankFileID || stmts[0].BankFileID == 0 {
		t.Fatalf("both statements must share one bank file: %d vs %d", stmts[0].BankFileID, stmts[1].BankFileID)
	}
	// Re-import returns the same two statements.
	again, err := stmtSvc.Import(ctx, account.ID, two, "carol")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(again) != 2 || again[0].ID != stmts[0].ID {
		t.Fatalf("re-import = %+v", again)
	}
}

func TestStatementImportValidation(t *testing.T) {
	ctx := context.Background()
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	if _, err := stmtSvc.Import(ctx, 1, ":20:A\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n", ""); !errors.Is(err, kasse.ErrActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	if _, err := stmtSvc.Import(ctx, 0, "x", "carol"); !errors.Is(err, kasse.ErrCashAccountRequired) {
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
	if _, err := stmtSvc.BankFile(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrBankFileNotFound) {
		t.Fatalf("bank file not found: %v", err)
	}
}

// TestStatementImportMatchesEndToEndRef covers reconciliation on the structured
// :86: end-to-end reference, plus the counterparty columns.
func TestStatementImportMatchesEndToEndRef(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	order := newApprovedOrder(t, orderSvc, 125000)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{order.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	content := ":20:STMT-" + uniqueSuffix(t) + "\n:25:DE02120300000000202051\n:28C:00001/001\n:60F:C261001EUR10000,00\n" +
		":61:2610011001D1250,00N051NONREF//BR\n" +
		":86:EREF+" + order.Reference + " SVWZ+Miete ABWA+Max Mustermann\n" +
		":62F:C261031EUR10000,00\n-\n"

	stmts, err := stmtSvc.Import(ctx, account.ID, content, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	l := stmts[0].Lines[0]
	if l.Status != kasse.StatusMatched || l.OrderID == nil || *l.OrderID != order.ID {
		t.Fatalf("line not matched via EREF: %+v", l)
	}
	if l.EndToEndRef != order.Reference || l.CounterpartyName != "Max Mustermann" || l.Purpose != "Miete" {
		t.Fatalf("line detail = %+v", l)
	}
}

// TestStatementImportCAMT053 covers format detection and reconciliation for a
// CAMT.053 document.
func TestStatementImportCAMT053(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	order := newApprovedOrder(t, orderSvc, 125000)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{order.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	doc := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08"><BkToCstmrStmt><Stmt>` +
		`<Id>CAMT-` + uniqueSuffix(t) + `</Id><ElctrncSeqNb>1</ElctrncSeqNb>` +
		`<Acct><Id><IBAN>DE02120300000000202051</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">10000.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Dt><Dt>2026-10-01</Dt></Dt></Bal>` +
		`<Ntry><Amt Ccy="EUR">1250.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>` +
		`<BookgDt><Dt>2026-10-01</Dt></BookgDt><ValDt><Dt>2026-10-01</Dt></ValDt>` +
		`<NtryDtls><TxDtls><Refs><EndToEndId>` + order.Reference + `</EndToEndId></Refs>` +
		`<RltdPties><Dbtr><Pty><Nm>Stadtwerke</Nm></Pty></Dbtr></RltdPties></TxDtls></NtryDtls></Ntry>` +
		`</Stmt></BkToCstmrStmt></Document>`

	stmts, err := stmtSvc.Import(ctx, account.ID, doc, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 || len(stmts[0].Lines) != 1 {
		t.Fatalf("stmts = %+v", stmts)
	}
	l := stmts[0].Lines[0]
	if l.Status != kasse.StatusMatched || l.OrderID == nil || *l.OrderID != order.ID {
		t.Fatalf("CAMT line not matched: %+v", l)
	}
	if l.CounterpartyName != "Stadtwerke" {
		t.Fatalf("counterparty = %q", l.CounterpartyName)
	}
}

func TestStatementImportCAMT054(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	order := newApprovedOrder(t, orderSvc, 125000)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{order.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// A camt.054 notification: the same entry set, no balance.
	doc := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.054.001.08"><BkToCstmrDbtCdtNtfctn><Ntfctn>` +
		`<Id>NTC-` + uniqueSuffix(t) + `</Id>` +
		`<Acct><Id><IBAN>DE02120300000000202051</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Ntry><Amt Ccy="EUR">1250.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>` +
		`<BookgDt><Dt>2026-10-28</Dt></BookgDt><ValDt><Dt>2026-10-28</Dt></ValDt>` +
		`<NtryDtls><TxDtls><Refs><EndToEndId>` + order.Reference + `</EndToEndId></Refs>` +
		`<RltdPties><Dbtr><Pty><Nm>Stadtwerke</Nm></Pty></Dbtr></RltdPties></TxDtls></NtryDtls></Ntry>` +
		`</Ntfctn></BkToCstmrDbtCdtNtfctn></Document>`

	stmts, err := stmtSvc.Import(ctx, account.ID, doc, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 || len(stmts[0].Lines) != 1 {
		t.Fatalf("stmts = %+v", stmts)
	}
	l := stmts[0].Lines[0]
	if l.Status != kasse.StatusMatched || l.OrderID == nil || *l.OrderID != order.ID {
		t.Fatalf("CAMT.054 line not matched: %+v", l)
	}
	if l.CounterpartyName != "Stadtwerke" {
		t.Fatalf("counterparty = %q", l.CounterpartyName)
	}

	// Idempotent on the file: re-import returns the stored statements.
	again, err := stmtSvc.Import(ctx, account.ID, doc, "carol")
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(again) != 1 || again[0].Reference != stmts[0].Reference {
		t.Fatalf("re-import not idempotent: %+v", again)
	}
}

func TestStatementImportCAMT052(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(testPool(t)), audit.NewStore(testPool(t)))

	order := newApprovedOrder(t, orderSvc, 125000)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{order.ID}, "carol"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// A camt.052 intraday account report: entries plus balances.
	doc := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.052.001.08"><BkToCstmrAcctRpt><Rpt>` +
		`<Id>RPT-` + uniqueSuffix(t) + `</Id><ElctrncSeqNb>1</ElctrncSeqNb>` +
		`<Acct><Id><IBAN>DE02120300000000202051</IBAN></Id><Ccy>EUR</Ccy></Acct>` +
		`<Bal><Tp><CdOrPrtry><Cd>OPBD</Cd></CdOrPrtry></Tp><Amt Ccy="EUR">0.00</Amt><CdtDbtInd>CRDT</CdtDbtInd><Dt><Dt>2026-10-28</Dt></Dt></Bal>` +
		`<Ntry><Amt Ccy="EUR">1250.00</Amt><CdtDbtInd>CRDT</CdtDbtInd>` +
		`<BookgDt><Dt>2026-10-28</Dt></BookgDt><ValDt><Dt>2026-10-28</Dt></ValDt>` +
		`<NtryDtls><TxDtls><Refs><EndToEndId>` + order.Reference + `</EndToEndId></Refs>` +
		`<RltdPties><Dbtr><Pty><Nm>Stadtwerke</Nm></Pty></Dbtr></RltdPties></TxDtls></NtryDtls></Ntry>` +
		`</Rpt></BkToCstmrAcctRpt></Document>`

	stmts, err := stmtSvc.Import(ctx, account.ID, doc, "carol")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(stmts) != 1 || len(stmts[0].Lines) != 1 {
		t.Fatalf("stmts = %+v", stmts)
	}
	l := stmts[0].Lines[0]
	if l.Status != kasse.StatusMatched || l.OrderID == nil || *l.OrderID != order.ID {
		t.Fatalf("CAMT.052 line not matched: %+v", l)
	}
	if l.CounterpartyName != "Stadtwerke" {
		t.Fatalf("counterparty = %q", l.CounterpartyName)
	}
}

// TestStatementAndCashAccountLists: the two reads a statements screen needs. Statements come back
// newest closing date first — the order an operator looks for one in — and the account filter
// narrows them on the server; accounts come back by name, which is how they are offered.
func TestStatementAndCashAccountLists(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	runSvc, _ := newRunService(t)
	stmtSvc := kasse.NewStatementService(kasse.NewStatementRepo(pool), audit.NewStore(pool))

	mark := uniqueSuffix(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni-Liste-"+mark, "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}

	// A closing date is what the list sorts on, so the statements are imported oldest first and
	// must come back the other way.
	file := func(ref, closing string) string {
		return ":20:" + ref + "\n:25:DE02120300000000202051\n:28C:00001/001\n" +
			":60F:C261001EUR10000,00\n:62F:C" + closing + "EUR10000,00\n-\n"
	}
	older, err := stmtSvc.Import(ctx, account.ID, file("STMT-OLD-"+mark, "261010"), "carol")
	if err != nil {
		t.Fatalf("Import older: %v", err)
	}
	newer, err := stmtSvc.Import(ctx, account.ID, file("STMT-NEW-"+mark, "261031"), "carol")
	if err != nil {
		t.Fatalf("Import newer: %v", err)
	}

	all, err := stmtSvc.Statements(ctx, 0)
	if err != nil {
		t.Fatalf("Statements: %v", err)
	}
	positions := map[int64]int{}
	for i, s := range all {
		positions[s.ID] = i
	}
	oldPos, haveOld := positions[older[0].ID]
	newPos, haveNew := positions[newer[0].ID]
	if !haveOld || !haveNew {
		t.Fatalf("both imported statements should be listed, got %d entries", len(all))
	}
	if newPos > oldPos {
		t.Fatalf("the newer statement (pos %d) is listed after the older one (pos %d)", newPos, oldPos)
	}

	// The summary carries what a list shows.
	for _, s := range all {
		if s.ID != newer[0].ID {
			continue
		}
		if s.AccountID != account.ID || s.Reference != "STMT-NEW-"+mark || s.Number != 1 {
			t.Fatalf("summary = %+v", s)
		}
		if s.OpeningBalanceCents != 1000000 || s.ClosingDate.Format(time.DateOnly) != "2026-10-31" {
			t.Fatalf("summary = %+v", s)
		}
		if s.ImportedBy != "carol" || s.LineCount != 0 {
			t.Fatalf("summary = %+v", s)
		}
	}

	// The filter narrows to one account.
	narrowed, err := stmtSvc.Statements(ctx, account.ID)
	if err != nil {
		t.Fatalf("Statements(account): %v", err)
	}
	if len(narrowed) < 2 {
		t.Fatalf("narrowed = %d, want the account's two statements", len(narrowed))
	}
	for _, s := range narrowed {
		if s.AccountID != account.ID {
			t.Fatalf("narrowed list contains a statement of account %d", s.AccountID)
		}
	}
	// An account id past the 32-bit range simply matches nothing rather than failing.
	if none, err := stmtSvc.Statements(ctx, 9223372036854775807); err != nil || len(none) != 0 {
		t.Fatalf("far account = %d statements, %v", len(none), err)
	}

	// The accounts come back by name.
	accounts, err := runSvc.CashAccounts(ctx)
	if err != nil {
		t.Fatalf("CashAccounts: %v", err)
	}
	at := -1
	for i, a := range accounts {
		if a.ID == account.ID {
			at = i
			if a.Name != "Uni-Liste-"+mark || a.IBAN == "" || a.Currency != "EUR" {
				t.Fatalf("account = %+v", a)
			}
		}
		if i > 0 && accounts[i-1].Name > a.Name {
			t.Fatalf("accounts are not sorted by name: %q before %q", accounts[i-1].Name, a.Name)
		}
	}
	if at < 0 {
		t.Fatalf("the account is not among %d", len(accounts))
	}
}

// TestStatementListToleratesMissingDates: a statement the bank sent without dates is still
// listed. The table allows them to be null, and a list that refuses to read a row it cannot date
// fails for every statement in the answer, not just that one — which is what it did before this
// test existed.
func TestStatementListToleratesMissingDates(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo := kasse.NewStatementRepo(pool)
	stmtSvc := kasse.NewStatementService(repo, audit.NewStore(pool))
	runSvc, _ := newRunService(t)

	mark := uniqueSuffix(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni-OhneDatum-"+mark, "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	file, err := repo.InsertBankFile(ctx, kasse.BankFile{
		AccountID: account.ID, FileName: "ohne-datum-" + mark, FileHash: "hash-" + mark, ImportedBy: "carol",
	})
	if err != nil {
		t.Fatalf("InsertBankFile: %v", err)
	}
	// A statement with no dates at all, which is what the zero time becomes on the way in.
	saved, err := repo.InsertStatements(ctx, file.ID, []kasse.Statement{{
		AccountID: account.ID, Reference: "STMT-NODATE-" + mark, Number: 1, Page: 1, Currency: "EUR",
		LineCount: 0, ImportedBy: "carol",
	}})
	if err != nil {
		t.Fatalf("InsertStatements: %v", err)
	}

	list, err := stmtSvc.Statements(ctx, account.ID)
	if err != nil {
		t.Fatalf("Statements: %v", err)
	}
	var found bool
	for _, s := range list {
		if s.ID != saved[0].ID {
			continue
		}
		found = true
		if !s.ClosingDate.IsZero() || !s.OpeningDate.IsZero() {
			t.Fatalf("a statement without dates came back dated: %+v", s)
		}
	}
	if !found {
		t.Fatalf("the undated statement is not among %d", len(list))
	}
}
