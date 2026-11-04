package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// stubBook satisfies the handler's cash-book interface and returns a fixed error,
// so the transport layer's unexpected-error branches are reachable without a DB.
type stubBook struct {
	day     kasse.Day
	booking kasse.Booking
	err     error
}

func (s stubBook) OpenDay(context.Context, time.Time, int64, string) (kasse.Day, error) {
	return s.day, s.err
}

func (s stubBook) Book(context.Context, int64, kasse.BookingKind, int64, string, string) (kasse.Booking, error) {
	return s.booking, s.err
}

func (s stubBook) CloseDay(context.Context, int64, string) (kasse.Day, error) {
	return s.day, s.err
}

func (s stubBook) Day(context.Context, int64) (kasse.Day, error) { return s.day, s.err }

type stubAudit struct{ err error }

func (s stubAudit) Verify(context.Context) error { return s.err }

// stubOrders satisfies the payment-order interface with a fixed result.
type stubOrders struct {
	order kasse.PaymentOrder
	err   error
}

func (s stubOrders) CreateRecipient(context.Context, kasse.Recipient) (kasse.Recipient, error) {
	return kasse.Recipient{ID: 1}, s.err
}

func (s stubOrders) CreateBudgetPosition(context.Context, kasse.BudgetPosition) (kasse.BudgetPosition, error) {
	return kasse.BudgetPosition{ID: 1}, s.err
}

func (s stubOrders) Create(context.Context, kasse.PaymentOrder) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

func (s stubOrders) Order(context.Context, int64) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

func (s stubOrders) Orders(context.Context, kasse.OrderStatus) ([]kasse.OrderSummary, error) {
	return nil, s.err
}

func (s stubOrders) Submit(context.Context, int64, string) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

func (s stubOrders) Approve(context.Context, int64, string) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

func (s stubOrders) Reject(context.Context, int64, string, string) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

func (s stubOrders) Cancel(context.Context, int64, string) (kasse.PaymentOrder, error) {
	return s.order, s.err
}

// stubDunning satisfies the dunning interface with a fixed result.
type stubDunning struct {
	level  kasse.DunningLevel
	notice kasse.DunningNotice
	err    error
}

func (s stubDunning) CreateLevel(context.Context, kasse.DunningLevel, string) (kasse.DunningLevel, error) {
	return s.level, s.err
}

func (s stubDunning) Level(context.Context, int, string) (kasse.DunningLevel, error) {
	return s.level, s.err
}

func (s stubDunning) Notice(context.Context, int64) (kasse.DunningNotice, error) {
	return s.notice, s.err
}

func (s stubDunning) Issue(context.Context, int, string, string, int64, string) (kasse.DunningNotice, error) {
	return s.notice, s.err
}

// stubTax satisfies the tax-data interface with a fixed result.
type stubTax struct {
	vat        kasse.VATType
	office     kasse.TaxOffice
	foundation kasse.Foundation
	err        error
}

func (s stubTax) CreateVATType(context.Context, kasse.VATType, string) (kasse.VATType, error) {
	return s.vat, s.err
}

func (s stubTax) VATType(context.Context, string) (kasse.VATType, error) {
	return s.vat, s.err
}

func (s stubTax) CreateTaxOffice(context.Context, kasse.TaxOffice, string) (kasse.TaxOffice, error) {
	return s.office, s.err
}

func (s stubTax) CreateFoundation(context.Context, kasse.Foundation, string) (kasse.Foundation, error) {
	return s.foundation, s.err
}

// stubRuns satisfies the payment-run interface with a fixed result.
type stubRuns struct {
	account kasse.CashAccount
	run     kasse.PaymentRun
	file    string
	err     error
}

func (s stubRuns) CreateCashAccount(context.Context, string, string, string, string) (kasse.CashAccount, error) {
	return s.account, s.err
}

func (s stubRuns) CashAccounts(context.Context) ([]kasse.CashAccount, error) {
	return nil, s.err
}

func (s stubRuns) CreateRun(context.Context, string, int64, time.Time, []int64, string) (kasse.PaymentRun, error) {
	return s.run, s.err
}

func (s stubRuns) Run(context.Context, int64) (kasse.PaymentRun, error) { return s.run, s.err }

func (s stubRuns) RunFile(context.Context, int64) (string, error) { return s.file, s.err }

// stubStatements satisfies the bank-statement interface with a fixed result.
type stubStatements struct {
	stmts []kasse.Statement
	st    kasse.Statement
	err   error
}

func (s stubStatements) Import(context.Context, int64, string, string) ([]kasse.Statement, error) {
	return s.stmts, s.err
}

func (s stubStatements) Statement(context.Context, int64) (kasse.Statement, error) {
	return s.st, s.err
}

func (s stubStatements) Statements(context.Context, int64) ([]kasse.StatementSummary, error) {
	return nil, s.err
}

func (s stubStatements) BankFile(context.Context, int64) (kasse.BankFile, error) {
	return kasse.BankFile{}, s.err
}

// stubFoundationBooks satisfies the foundation-book interface with a fixed result.
type stubFoundationBooks struct{ err error }

func (s stubFoundationBooks) CreateBook(context.Context, int64, string, string, string) (kasse.FoundationBook, error) {
	return kasse.FoundationBook{}, s.err
}

func (s stubFoundationBooks) Book(context.Context, int64, int64, kasse.BookingKind, string, string) (kasse.FoundationBooking, error) {
	return kasse.FoundationBooking{}, s.err
}

func (s stubFoundationBooks) Bookings(context.Context, int64) ([]kasse.FoundationBooking, error) {
	return nil, s.err
}

func (s stubFoundationBooks) Balance(context.Context, int64) (kasse.FoundationBalance, error) {
	return kasse.FoundationBalance{}, s.err
}

// stubEInvoices satisfies the e-invoice interface with a fixed result.
type stubEInvoices struct{ err error }

func (s stubEInvoices) Create(context.Context, core.Invoice, string) (core.Invoice, error) {
	return core.Invoice{}, s.err
}

func (s stubEInvoices) ImportReceived(context.Context, string, string) (core.Invoice, error) {
	return core.Invoice{}, s.err
}

func (s stubEInvoices) Get(context.Context, int64) (core.Invoice, error) {
	return core.Invoice{}, s.err
}

func (s stubEInvoices) Render(context.Context, int64) (string, error) {
	return "", s.err
}

func (s stubEInvoices) Match(context.Context, int64, string) (core.Invoice, error) {
	return core.Invoice{}, s.err
}

func (s stubEInvoices) Invoices(context.Context) ([]einvoice.InvoiceSummary, error) {
	return nil, s.err
}

// stubBudget satisfies the budget interface with a fixed result.
type stubBudget struct{ err error }

func (s stubBudget) CreatePosition(context.Context, int, string, string, string, int64, string) (budget.Position, error) {
	return budget.Position{}, s.err
}

func (s stubBudget) Post(context.Context, int64, budget.PostingKind, int64, string, string) (budget.Posting, error) {
	return budget.Posting{}, s.err
}

func (s stubBudget) GetPosition(context.Context, int64) (budget.Position, error) {
	return budget.Position{}, s.err
}

func (s stubBudget) Positions(context.Context) ([]budget.Position, error) {
	return nil, s.err
}

func (s stubBudget) Balance(context.Context, int64) (budget.Balance, error) {
	return budget.Balance{}, s.err
}

// stubLookups satisfies the lookup interface with a fixed result.
type stubLookups struct{ err error }

func (s stubLookups) Upsert(context.Context, lookup.Value, string) (lookup.Value, error) {
	return lookup.Value{}, s.err
}

func (s stubLookups) UpsertMany(context.Context, []lookup.Value, string) ([]lookup.Value, error) {
	return nil, s.err
}

func (s stubLookups) Get(context.Context, string, string) (lookup.Value, error) {
	return lookup.Value{}, s.err
}

func (s stubLookups) List(context.Context, string) ([]lookup.Value, error) {
	return nil, s.err
}

func (s stubLookups) Seed(context.Context, string) (int, error) {
	return 0, s.err
}

// stubProcurement satisfies the purchase-order interface with a fixed result.
type stubProcurement struct{ err error }

func (s stubProcurement) Create(context.Context, string, string, int64, string, []procurement.Line, string) (procurement.Order, error) {
	return procurement.Order{}, s.err
}

func (s stubProcurement) Approve(context.Context, int64, string) (procurement.Order, error) {
	return procurement.Order{}, s.err
}

func (s stubProcurement) Receive(context.Context, int64, string) (procurement.Order, error) {
	return procurement.Order{}, s.err
}

func (s stubProcurement) Cancel(context.Context, int64, string) (procurement.Order, error) {
	return procurement.Order{}, s.err
}

func (s stubProcurement) Get(context.Context, int64) (procurement.Order, error) {
	return procurement.Order{}, s.err
}

func (s stubProcurement) Orders(context.Context, procurement.Status) ([]procurement.OrderSummary, error) {
	return nil, s.err
}

// stubKLR satisfies the KLR interface with a fixed result.
type stubKLR struct{ err error }

func (s stubKLR) CreateCostCenter(context.Context, string, string, string) (klr.CostCenter, error) {
	return klr.CostCenter{}, s.err
}

func (s stubKLR) CostCenters(context.Context) ([]klr.CostCenter, error) {
	return nil, s.err
}

func (s stubKLR) CreateCostUnit(context.Context, string, string, string) (klr.CostUnit, error) {
	return klr.CostUnit{}, s.err
}

func (s stubKLR) Post(context.Context, int64, *int64, *int64, klr.Kind, int64, time.Time, string, string) (klr.Record, error) {
	return klr.Record{}, s.err
}

func (s stubKLR) PostCostType(context.Context, string, string, string) (klr.CostType, error) {
	return klr.CostType{}, s.err
}

func (s stubKLR) DefineCostUnitKey(context.Context, int64, int64, int64, time.Time, *time.Time, string) (klr.CostUnitKey, error) {
	return klr.CostUnitKey{}, s.err
}

func (s stubKLR) CostUnitKeys(context.Context, int64) ([]klr.CostUnitKey, error) {
	return nil, s.err
}

func (s stubKLR) CostUnits(context.Context) ([]klr.CostUnit, error) {
	return nil, s.err
}

func (s stubKLR) AllocateToCostUnits(context.Context, int64, time.Time, time.Time, string) (klr.Run, error) {
	return klr.Run{}, s.err
}

func (s stubKLR) CostTypes(context.Context) ([]klr.CostType, error) {
	return nil, s.err
}

func (s stubKLR) MapAccount(context.Context, int64, string, string) (klr.CostTypeAccount, error) {
	return klr.CostTypeAccount{}, s.err
}

func (s stubKLR) AccountsOf(context.Context, int64) ([]klr.CostTypeAccount, error) {
	return nil, s.err
}

func (s stubKLR) CostTypeByAccount(context.Context, string) (klr.CostType, error) {
	return klr.CostType{}, s.err
}

func (s stubKLR) DefineAllocation(context.Context, int64, int64, int64, time.Time, *time.Time, string) (klr.Allocation, error) {
	return klr.Allocation{}, s.err
}

func (s stubKLR) Allocations(context.Context, int64) ([]klr.Allocation, error) {
	return nil, s.err
}

func (s stubKLR) Allocate(context.Context, int64, time.Time, time.Time, string) (klr.Run, error) {
	return klr.Run{}, s.err
}

func (s stubKLR) Report(context.Context, int64) (klr.Report, error) {
	return klr.Report{}, s.err
}

func TestHTTPUnexpectedErrorsReturn500(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Kasse: stubBook{err: errors.New("boom")}, Orders: stubOrders{}, Dunning: stubDunning{}, Tax: stubTax{}, Runs: stubRuns{}, Statements: stubStatements{}, FoundationBooks: stubFoundationBooks{}, EInvoices: stubEInvoices{}, Budget: stubBudget{}, Lookups: stubLookups{}, Procurement: stubProcurement{}, KLR: stubKLR{}, Audit: stubAudit{}}).Handler())
	defer srv.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"get day", http.MethodGet, "/api/cashbook/days/1", ""},
		{"book", http.MethodPost, "/api/cashbook/days/1/bookings", `{"kind":"income","amount_cents":5,"purpose":"x"}`},
		{"close", http.MethodPost, "/api/cashbook/days/1/close", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if resp := doJSON(t, c.method, srv.URL+c.path, "alice", c.body); resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("%s %s = %d, want 500", c.method, c.path, resp.StatusCode)
			}
		})
	}
}

func TestHTTPAuditVerifyFailureReturns500(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Kasse: stubBook{}, Orders: stubOrders{}, Dunning: stubDunning{}, Tax: stubTax{}, Runs: stubRuns{}, Statements: stubStatements{}, FoundationBooks: stubFoundationBooks{}, EInvoices: stubEInvoices{}, Budget: stubBudget{}, Lookups: stubLookups{}, Procurement: stubProcurement{}, KLR: stubKLR{}, Audit: stubAudit{err: errors.New("chain broken")}}).Handler())
	defer srv.Close()
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/audit/verify", "", ""); resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("audit verify = %d, want 500", resp.StatusCode)
	}
}

func TestHTTPHealthAndNotFound(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{Kasse: stubBook{err: kasse.ErrNotFound}, Orders: stubOrders{}, Dunning: stubDunning{}, Tax: stubTax{}, Runs: stubRuns{}, Statements: stubStatements{}, FoundationBooks: stubFoundationBooks{}, EInvoices: stubEInvoices{}, Budget: stubBudget{}, Lookups: stubLookups{}, Procurement: stubProcurement{}, KLR: stubKLR{}, Audit: stubAudit{}}).Handler())
	defer srv.Close()
	if resp := doJSON(t, http.MethodGet, srv.URL+"/healthz", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/cashbook/days/1", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get day = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/1/close", "a", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("close = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/1/bookings", "a",
		`{"kind":"income","amount_cents":5,"purpose":"x"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("book = %d, want 404", resp.StatusCode)
	}
}
