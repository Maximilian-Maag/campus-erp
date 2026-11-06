package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	level    kasse.DunningLevel
	notice   kasse.DunningNotice
	err      error
	seenYear *int
}

func (s stubDunning) CreateLevel(context.Context, kasse.DunningLevel, string) (kasse.DunningLevel, error) {
	return s.level, s.err
}

func (s stubDunning) Level(context.Context, int, string) (kasse.DunningLevel, error) {
	return s.level, s.err
}

func (s stubDunning) Levels(_ context.Context, year int) ([]kasse.DunningLevel, error) {
	if s.seenYear != nil {
		*s.seenYear = year
	}
	if s.err != nil {
		return nil, s.err
	}
	return []kasse.DunningLevel{s.level}, nil
}

func (s stubDunning) Notice(context.Context, int64) (kasse.DunningNotice, error) {
	return s.notice, s.err
}

func (s stubDunning) Issue(context.Context, int, string, string, int64, string) (kasse.DunningNotice, error) {
	return s.notice, s.err
}

// stubTax satisfies the tax-data interface with a fixed result.
type stubTax struct {
	vat         kasse.VATType
	office      kasse.TaxOffice
	foundation  kasse.Foundation
	foundations []kasse.Foundation
	seenYear    *int
	err         error
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

func (s stubTax) Foundations(_ context.Context, year int) ([]kasse.Foundation, error) {
	if s.seenYear != nil {
		*s.seenYear = year
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.foundations, nil
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
type stubFoundationBooks struct {
	books          []kasse.FoundationBook
	seenFoundation *int64
	err            error
}

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

func (s stubFoundationBooks) Books(_ context.Context, foundationID int64) ([]kasse.FoundationBook, error) {
	if s.seenFoundation != nil {
		*s.seenFoundation = foundationID
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.books, nil
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
type stubBudget struct {
	position budget.Position
	err      error
}

func (s stubBudget) CreatePosition(context.Context, int, string, string, string, int64, string) (budget.Position, error) {
	return budget.Position{}, s.err
}

func (s stubBudget) Post(context.Context, int64, budget.PostingKind, int64, string, string) (budget.Posting, error) {
	return budget.Posting{}, s.err
}

func (s stubBudget) GetPosition(context.Context, int64) (budget.Position, error) {
	return s.position, s.err
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

func (s stubKLR) DefineServiceRate(context.Context, int64, int64, time.Time, *time.Time, string) (klr.ServiceRate, error) {
	return klr.ServiceRate{}, s.err
}

func (s stubKLR) ServiceRates(context.Context) ([]klr.ServiceRate, error) {
	return nil, s.err
}

func (s stubKLR) ChargeService(context.Context, int64, int64, int64, int64, time.Time, string) (klr.ServiceCharge, error) {
	return klr.ServiceCharge{}, s.err
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

// TestListDunningLevelsYearQuery covers the levels list without a database: the year is
// the server's to parse, so a non-integer is the caller's mistake and no year means
// every year.
func TestListDunningLevelsYearQuery(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		want     int
		wantYear int
	}{
		{"no year lists every year", "", http.StatusOK, 0},
		{"one year", "?year=2026", http.StatusOK, 2026},
		{"a padded year is still that year", "?year=%202026", http.StatusOK, 2026},
		{"a non-integer year is a client error", "?year=abc", http.StatusUnprocessableEntity, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seen int
			handler := httpapi.New(httpapi.Deps{
				Dunning: stubDunning{level: kasse.DunningLevel{Code: "1"}, seenYear: &seen},
			}).Handler()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dunning/levels"+c.query, nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.want == http.StatusOK {
				if seen != c.wantYear {
					t.Fatalf("service saw year %d, want %d", seen, c.wantYear)
				}
				if !strings.Contains(rec.Body.String(), `"Code":"1"`) {
					t.Fatalf("body = %s, want the level", rec.Body.String())
				}
			}
		})
	}
}

// TestListFoundationsAndBooksWithoutDatabase covers the two reads the Stiftungen screen opens
// with, without a database: ?year= is parsed by the server (no year lists every year, a padded
// one is read as its number, a non-number is refused by code), and a foundation's books are
// read from the id in the path — never from anything a caller could vary.
func TestListFoundationsAndBooksWithoutDatabase(t *testing.T) {
	foundation := kasse.Foundation{ID: 4, Year: 2026, Number: "1"}
	book := kasse.FoundationBook{ID: 9, FoundationID: 4, Reference: "B-1", Currency: "EUR"}

	cases := []struct {
		name     string
		query    string
		want     int
		wantYear int
	}{
		{"no year lists every year", "", http.StatusOK, 0},
		{"one year", "?year=2026", http.StatusOK, 2026},
		{"a padded year is still that year", "?year=%202026", http.StatusOK, 2026},
		{"a non-integer year is a client error", "?year=abc", http.StatusUnprocessableEntity, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seenYear int
			handler := httpapi.New(httpapi.Deps{
				Tax: stubTax{foundations: []kasse.Foundation{foundation}, seenYear: &seenYear},
			}).Handler()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/foundations"+c.query, nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.want != http.StatusOK {
				return
			}
			if seenYear != c.wantYear {
				t.Fatalf("service saw year %d, want %d", seenYear, c.wantYear)
			}
			if !strings.Contains(rec.Body.String(), `"Number":"1"`) {
				t.Fatalf("body = %s, want the foundation", rec.Body.String())
			}
		})
	}

	t.Run("books of the foundation in the path", func(t *testing.T) {
		var seenFoundation int64
		handler := httpapi.New(httpapi.Deps{
			FoundationBooks: stubFoundationBooks{books: []kasse.FoundationBook{book}, seenFoundation: &seenFoundation},
		}).Handler()

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/foundations/4/books", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if seenFoundation != 4 {
			t.Fatalf("service saw foundation %d, want 4", seenFoundation)
		}
		if !strings.Contains(rec.Body.String(), `"Reference":"B-1"`) {
			t.Fatalf("body = %s, want the book", rec.Body.String())
		}
	})

	t.Run("a path that is not an id is refused before the service", func(t *testing.T) {
		var seenFoundation int64
		handler := httpapi.New(httpapi.Deps{
			FoundationBooks: stubFoundationBooks{seenFoundation: &seenFoundation},
		}).Handler()

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/foundations/abc/books", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if seenFoundation != 0 {
			t.Fatalf("the service was asked for foundation %d", seenFoundation)
		}
	})

	t.Run("a failing service is a server error", func(t *testing.T) {
		handler := httpapi.New(httpapi.Deps{
			Tax:             stubTax{err: errors.New("boom")},
			FoundationBooks: stubFoundationBooks{err: errors.New("boom")},
		}).Handler()

		for _, url := range []string{"/api/foundations", "/api/foundations/4/books"} {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("GET %s = %d, want 500", url, rec.Code)
			}
		}
	})
}

// orderBodyWithPosition is a complete order body whose `budget_position` is the caller's.
func orderBodyWithPosition(budgetPosition string) string {
	return `{"reference":"AO-1","business_year":2026,"amount_cents":100,"currency":"EUR",` +
		`"purpose":"x","due_date":"2026-04-01",` +
		`"recipient":{"name":"n","account_holder":"h","iban":"DE02120300000000202051",` +
		`"bic":"BYLADEM1001","bank_name":"b"},` +
		`"budget_position":` + budgetPosition + `}`
}

// TestCreateOrderBudgetPositionWithoutDatabase covers how an order's budget position is
// resolved without a database: an id names an existing one (through the budget module), a
// description creates one inline, and a body that does both is refused.
func TestCreateOrderBudgetPositionWithoutDatabase(t *testing.T) {
	withBudget := func(p budget.Position, err error) httpapi.Deps {
		return httpapi.Deps{Orders: stubOrders{}, Budget: stubBudget{position: p, err: err}}
	}
	named := withBudget(budget.Position{ID: 7}, nil)

	cases := []struct {
		name     string
		position string
		deps     httpapi.Deps
		want     int
		wantCode string
	}{
		{"names an existing position", `{"id":7}`, named, http.StatusCreated, ""},
		{"describes a new position", `{"year":2026,"chapter":"0710","title":"6810","cost_centre":"KST","cost_carrier":"KT","allocated_cents":500000}`, withBudget(budget.Position{}, nil), http.StatusCreated, ""},
		// Each field of a description counts on its own: naming an id and setting any one of
		// them is the ambiguous body, not a description to be merged into the named position.
		{"both: year", `{"id":7,"year":2026}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"both: chapter", `{"id":7,"chapter":"0710"}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"both: title", `{"id":7,"title":"6810"}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"both: cost centre", `{"id":7,"cost_centre":"KST"}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"both: cost carrier", `{"id":7,"cost_carrier":"KT"}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"both: allocation", `{"id":7,"allocated_cents":1}`, named, http.StatusUnprocessableEntity, "budget_position_ambiguous"},
		{"an id nothing carries", `{"id":7}`, withBudget(budget.Position{}, budget.ErrPositionNotFound), http.StatusNotFound, ""},
		{"the budget module fails", `{"id":7}`, withBudget(budget.Position{}, errors.New("boom")), http.StatusInternalServerError, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httpapi.New(c.deps).Handler().ServeHTTP(rec,
				httptest.NewRequest(http.MethodPost, "/api/payment-orders", strings.NewReader(orderBodyWithPosition(c.position))))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
			if c.wantCode == "" {
				return
			}
			var problem struct{ Code string }
			if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Code != c.wantCode {
				t.Fatalf("code = %q, want %q", problem.Code, c.wantCode)
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
