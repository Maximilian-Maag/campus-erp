// Package httpapi exposes the backend's HTTP surface. Handlers stay thin: they
// decode, delegate to a domain service, and map errors to status codes.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/storage"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/tenant"
)

// cashBook is the cash-book use-case surface the HTTP layer needs. It is an
// interface so the transport layer can be tested with stubs, independent of the
// database.
type cashBook interface {
	OpenDay(ctx context.Context, date time.Time, openingCents int64, by string) (kasse.Day, error)
	Book(ctx context.Context, dayID int64, kind kasse.BookingKind, amountCents int64, purpose, by string) (kasse.Booking, error)
	CloseDay(ctx context.Context, dayID int64, by string) (kasse.Day, error)
	Day(ctx context.Context, id int64) (kasse.Day, error)
}

// auditVerifier recomputes the audit chain.
type auditVerifier interface {
	Verify(ctx context.Context) error
}

// readyTimeout bounds the readiness database ping.
const readyTimeout = 2 * time.Second

// healthChecker reports whether a backing dependency (the database) is reachable.
// Readiness must not be a constant "ok": a load balancer has to know the process
// can actually serve (NFR-6).
type healthChecker interface {
	Ping(ctx context.Context) error
}

// Server wires HTTP handlers to the domain services.
type Server struct {
	kasse           cashBook
	orders          orderService
	dunning         dunningService
	tax             taxService
	runs            paymentRunService
	statements      statementService
	foundationBooks foundationBookService
	einvoices       einvoiceService
	budget          budgetService
	lookups         lookupService
	procurement     procurementService
	klr             klrService
	mandates        mandateService
	receivables     receivableService
	collectionRuns  collectionRunService
	etl             etlRunner
	audit           auditVerifier
	health          healthChecker
	auth            auth.Verifier
	artefacts       storage.Store
	exports         exportQueue
	reports         reportCatalogue
}

// Deps are the collaborators the HTTP server needs. A struct (not thirteen
// positional interfaces) keeps the constructor readable and lets tests supply
// only the collaborators a given case exercises; the zero value of each field is
// a nil interface that panics only if that route is called.
type Deps struct {
	Kasse           cashBook
	Orders          orderService
	Dunning         dunningService
	Tax             taxService
	Runs            paymentRunService
	Statements      statementService
	FoundationBooks foundationBookService
	EInvoices       einvoiceService
	Budget          budgetService
	Lookups         lookupService
	Procurement     procurementService
	KLR             klrService
	Mandates        mandateService
	Receivables     receivableService
	CollectionRuns  collectionRunService
	ETL             etlRunner
	Audit           auditVerifier
	Health          healthChecker
	Auth            auth.Verifier
	Artefacts       storage.Store
	Exports         exportQueue
	Reports         reportCatalogue
}

// New returns a Server.
func New(d Deps) *Server {
	return &Server{
		kasse:           d.Kasse,
		orders:          d.Orders,
		dunning:         d.Dunning,
		tax:             d.Tax,
		runs:            d.Runs,
		statements:      d.Statements,
		foundationBooks: d.FoundationBooks,
		einvoices:       d.EInvoices,
		budget:          d.Budget,
		lookups:         d.Lookups,
		procurement:     d.Procurement,
		klr:             d.KLR,
		mandates:        d.Mandates,
		receivables:     d.Receivables,
		collectionRuns:  d.CollectionRuns,
		etl:             d.ETL,
		audit:           d.Audit,
		health:          d.Health,
		auth:            d.Auth,
		artefacts:       d.Artefacts,
		exports:         d.Exports,
		reports:         d.Reports,
	}
}

// Handler returns the routed HTTP handler (cross-cutting middleware included).
func (s *Server) Handler() http.Handler {
	return middleware(s.Routes(), slog.Default())
}

// Routes returns the authenticated route table without the outer middleware, so
// a multi-tenant front (Tenancy) can compose it under its own access log.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Health probes stay open: a load balancer and tooling must reach them without
	// a token, so they sit outside /api/ and outside authenticate.
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)

	// Role model (ADR-0003): each route is owned by a module role, distinguished
	// read from write. erp-admin may call any route; erp-reader may call any read.
	// The fine-grained legacy sv_rechte_sachb model maps onto these roles at the
	// IdP, so the API only enforces the coarse module boundary.
	mux.HandleFunc("GET /api/audit/verify", s.authorize(s.auditVerify, auth.RoleAdmin))
	mux.HandleFunc("GET /api/artefacts/{kind}/{name}", s.authorizeRead(s.getArtefact, auth.RoleKasse))

	// Reports render on the worker role; the API lists them from the same catalogue (docs/25).
	mux.HandleFunc("GET /api/reports", s.authorizeRead(s.listReports, auth.RoleKasse))
	// Exports run on the worker role; the API only queues them (docs/27).
	mux.HandleFunc("POST /api/statistics/exports", s.authorize(s.requestStatisticsExport, auth.RoleKasse))
	mux.HandleFunc("POST /api/ledger/exports", s.authorize(s.requestLedgerExport, auth.RoleKasse))
	mux.HandleFunc("POST /api/reports/renders", s.authorize(s.requestReportRender, auth.RoleKasse))
	mux.HandleFunc("POST /api/bank-statements/imports", s.authorize(s.requestStatementImport, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-runs/{id}/exports", s.authorize(s.requestSepaExport, auth.RoleKasse))

	// ETL: list + run the registered datasets (admin).
	mux.HandleFunc("GET /api/etl/datasets", s.authorize(s.listETLDatasets, auth.RoleAdmin))
	mux.HandleFunc("GET /api/etl/runs", s.authorize(s.listETLRuns, auth.RoleAdmin))
	mux.HandleFunc("POST /api/etl/runs", s.authorize(s.runETLDataset, auth.RoleAdmin))

	mux.HandleFunc("POST /api/cashbook/days", s.authorize(s.openDay, auth.RoleKasse))
	mux.HandleFunc("GET /api/cashbook/days/{id}", s.authorizeRead(s.getDay, auth.RoleKasse))
	mux.HandleFunc("POST /api/cashbook/days/{id}/bookings", s.authorize(s.book, auth.RoleKasse))
	mux.HandleFunc("POST /api/cashbook/days/{id}/close", s.authorize(s.closeDay, auth.RoleKasse))

	mux.HandleFunc("POST /api/payment-orders", s.authorize(s.createOrder, auth.RoleKasse))
	mux.HandleFunc("GET /api/payment-orders", s.authorizeRead(s.listOrders, auth.RoleKasse))
	mux.HandleFunc("GET /api/payment-orders/{id}", s.authorizeRead(s.getOrder, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-orders/{id}/submit", s.authorize(s.submitOrder, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-orders/{id}/approve", s.authorize(s.approveOrder, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-orders/{id}/reject", s.authorize(s.rejectOrder, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-orders/{id}/cancel", s.authorize(s.cancelOrder, auth.RoleKasse))

	mux.HandleFunc("POST /api/cash-accounts", s.authorize(s.createCashAccount, auth.RoleKasse))
	mux.HandleFunc("GET /api/cash-accounts", s.authorizeRead(s.listCashAccounts, auth.RoleKasse))
	mux.HandleFunc("POST /api/payment-runs", s.authorize(s.createRun, auth.RoleKasse))
	mux.HandleFunc("GET /api/payment-runs/{id}", s.authorizeRead(s.getRun, auth.RoleKasse))
	mux.HandleFunc("GET /api/payment-runs/{id}/file", s.authorizeRead(s.getRunFile, auth.RoleKasse))

	mux.HandleFunc("POST /api/bank-statements", s.authorize(s.importStatement, auth.RoleKasse))
	mux.HandleFunc("GET /api/bank-statements", s.authorizeRead(s.listStatements, auth.RoleKasse))
	mux.HandleFunc("GET /api/bank-statements/{id}", s.authorizeRead(s.getStatement, auth.RoleKasse))
	mux.HandleFunc("GET /api/bank-files/{id}", s.authorizeRead(s.getBankFile, auth.RoleKasse))

	mux.HandleFunc("POST /api/foundation-books", s.authorize(s.createFoundationBook, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/foundation-books/{id}/bookings", s.authorize(s.bookFoundation, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/foundation-books/{id}/bookings", s.authorizeRead(s.getFoundationBookings, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/foundation-books/{id}/balance", s.authorizeRead(s.getFoundationBalance, auth.RoleFinanzen))

	mux.HandleFunc("POST /api/e-invoices", s.authorize(s.createEInvoice, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/e-invoices/received", s.authorize(s.receiveEInvoice, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/e-invoices", s.authorizeRead(s.listEInvoices, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/e-invoices/{id}", s.authorizeRead(s.getEInvoice, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/e-invoices/{id}/xrechnung", s.authorizeRead(s.getEInvoiceXML, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/e-invoices/{id}/match", s.authorize(s.matchEInvoice, auth.RoleFinanzen))

	mux.HandleFunc("POST /api/budget/positions", s.authorize(s.createBudgetPosition, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/budget/positions", s.authorizeRead(s.listBudgetPositions, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/budget/positions/{id}", s.authorizeRead(s.getBudgetPosition, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/budget/positions/{id}/postings", s.authorize(s.postBudget, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/budget/positions/{id}/balance", s.authorizeRead(s.getBudgetBalance, auth.RoleFinanzen))

	// The lookup catalogue is shared reference data: reads are open to any
	// authenticated caller, writes are administrative.
	mux.HandleFunc("POST /api/lookups/seed", s.authorize(s.seedLookups, auth.RoleAdmin))
	mux.HandleFunc("GET /api/lookups/{namespace}", s.authorize(s.listLookups))
	mux.HandleFunc("GET /api/lookups/{namespace}/{code}", s.authorize(s.getLookup))
	mux.HandleFunc("POST /api/lookups/{namespace}", s.authorize(s.upsertLookups, auth.RoleAdmin))

	mux.HandleFunc("POST /api/purchase-orders", s.authorize(s.createPurchaseOrder, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/purchase-orders", s.authorizeRead(s.listPurchaseOrders, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/purchase-orders/{id}", s.authorizeRead(s.getPurchaseOrder, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/purchase-orders/{id}/approve", s.authorize(s.approvePurchaseOrder, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/purchase-orders/{id}/receive", s.authorize(s.receivePurchaseOrder, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/purchase-orders/{id}/cancel", s.authorize(s.cancelPurchaseOrder, auth.RoleFinanzen))

	mux.HandleFunc("POST /api/klr/cost-centers", s.authorize(s.createCostCenter, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-centers", s.authorizeRead(s.listCostCenters, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-units", s.authorize(s.createCostUnit, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-units", s.authorizeRead(s.listCostUnits, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/records", s.authorize(s.postAccountingRecord, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-centers/{id}/report", s.authorizeRead(s.getKLRReport, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/allocations", s.authorize(s.defineAllocation, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-centers/{id}/allocations", s.authorizeRead(s.listAllocations, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/allocations/run", s.authorize(s.runAllocation, auth.RoleKLR))

	// Kostenträgerrechnung: the same distribution, with cost units as the receivers.
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/cost-unit-keys", s.authorize(s.defineCostUnitKey, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-centers/{id}/cost-unit-keys", s.authorizeRead(s.listCostUnitKeys, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/cost-unit-keys/run", s.authorize(s.runCostUnitAllocation, auth.RoleKLR))

	// Internal pricing: a rate per cost type, and the charges it prices.
	mux.HandleFunc("POST /api/klr/service-rates", s.authorize(s.defineServiceRate, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/service-rates", s.authorizeRead(s.listServiceRates, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/service-charges", s.authorize(s.chargeService, auth.RoleKLR))

	// Cost types (Kostenarten) and the account bridge from the ledger's accounts.
	mux.HandleFunc("POST /api/klr/cost-types", s.authorize(s.createCostType, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-types", s.authorizeRead(s.listCostTypes, auth.RoleKLR))
	mux.HandleFunc("POST /api/klr/cost-types/{id}/accounts", s.authorize(s.mapCostTypeAccount, auth.RoleKLR))
	mux.HandleFunc("GET /api/klr/cost-types/{id}/accounts", s.authorizeRead(s.listCostTypeAccounts, auth.RoleKLR))
	// The bridge's read side lives on the account, not under the cost type: "/cost-types/by-account/
	// {account}" would be as specific as "/cost-types/{id}/accounts" and the mux refuses to guess.
	mux.HandleFunc("GET /api/klr/accounts/{account}/cost-type", s.authorizeRead(s.getCostTypeByAccount, auth.RoleKLR))
	mux.HandleFunc("POST /api/dunning/levels", s.authorize(s.createDunningLevel, auth.RoleKasse))
	mux.HandleFunc("GET /api/dunning/levels", s.authorizeRead(s.listDunningLevels, auth.RoleKasse))
	mux.HandleFunc("GET /api/dunning/levels/{year}/{code}", s.authorizeRead(s.getDunningLevel, auth.RoleKasse))
	mux.HandleFunc("GET /api/dunning/notices/{id}/document", s.authorizeRead(s.getDunningNoticeDocument, auth.RoleKasse))
	mux.HandleFunc("POST /api/dunning/notices", s.authorize(s.issueDunningNotice, auth.RoleKasse))

	mux.HandleFunc("POST /api/vat-types", s.authorize(s.createVATType, auth.RoleFinanzen))
	mux.HandleFunc("GET /api/vat-types/{code}", s.authorizeRead(s.getVATType, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/tax-offices", s.authorize(s.createTaxOffice, auth.RoleFinanzen))
	mux.HandleFunc("POST /api/foundations", s.authorize(s.createFoundation, auth.RoleFinanzen))

	// SEPA direct-debit mandates (receivables — Kasse, like dunning).
	mux.HandleFunc("POST /api/direct-debit/mandates", s.authorize(s.createMandate, auth.RoleKasse))
	mux.HandleFunc("GET /api/direct-debit/mandates", s.authorizeRead(s.listMandates, auth.RoleKasse))
	mux.HandleFunc("GET /api/direct-debit/mandates/{reference}", s.authorizeRead(s.getMandate, auth.RoleKasse))
	mux.HandleFunc("POST /api/direct-debit/runs", s.authorize(s.createCollectionRun, auth.RoleKasse))
	mux.HandleFunc("POST /api/direct-debit/runs/from-receivables", s.authorize(s.createCollectionRunFromReceivables, auth.RoleKasse))
	mux.HandleFunc("GET /api/direct-debit/runs/{reference}/file", s.authorizeRead(s.getCollectionRunFile, auth.RoleKasse))

	// Open receivables a collection run selects from (Kasse).
	mux.HandleFunc("POST /api/receivables", s.authorize(s.createReceivable, auth.RoleKasse))
	mux.HandleFunc("GET /api/receivables", s.authorizeRead(s.listReceivables, auth.RoleKasse))
	mux.HandleFunc("GET /api/receivables/{id}", s.authorizeRead(s.getReceivable, auth.RoleKasse))
	// authenticate runs first (it attaches the principal); authorize reads it.
	return s.authenticate(mux)
}

// actor is the caller recorded on audit events: the authenticated subject when a
// token was presented, otherwise the dev X-Actor header, otherwise "anonymous",
// qualified by the tenant the request was resolved for. Recording the tenant on every
// event makes an audit row say which tenant it belongs to without a schema change
// (ADR-0002, docs/17).
func actor(r *http.Request) string {
	return withTenant(r.Context(), caller(r))
}

// caller is the caller identity, before the tenant is qualified onto it.
func caller(r *http.Request) string {
	if p, ok := principalFrom(r.Context()); ok && p.Subject != "" {
		return p.Subject
	}
	if a := r.Header.Get("X-Actor"); a != "" {
		return a
	}
	return "anonymous"
}

// withTenant prefixes the actor with the resolved tenant. A server built without the
// tenancy front — as the handler tests do — has no tenant in context, so the actor is
// left as it was.
func withTenant(ctx context.Context, who string) string {
	if name, ok := tenant.From(ctx); ok && name != "" {
		return name + "/" + who
	}
	return who
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	// Liveness: the process is up. Deliberately does not touch the database.
	_, _ = w.Write([]byte("ok"))
}

// readyz reports readiness: it pings the backing dependency so a load balancer
// only routes to a process that can serve (NFR-6).
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.health == nil {
		writeProblem(w, http.StatusServiceUnavailable, "no health checker configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()
	if err := s.health.Ping(ctx); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) auditVerify(w http.ResponseWriter, r *http.Request) {
	if err := s.audit.Verify(r.Context()); err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = w.Write([]byte("ok"))
}

type openDayRequest struct {
	BusinessDate string `json:"business_date"`
	OpeningCents int64  `json:"opening_cents"`
}

func (s *Server) openDay(w http.ResponseWriter, r *http.Request) {
	var req openDayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	date, err := time.Parse("2006-01-02", req.BusinessDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "business_date must be YYYY-MM-DD")
		return
	}
	day, err := s.kasse.OpenDay(r.Context(), date, req.OpeningCents, actor(r))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, day)
}

func (s *Server) getDay(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	day, err := s.kasse.Day(r.Context(), id)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, day)
}

type bookRequest struct {
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Purpose     string `json:"purpose"`
}

func (s *Server) book(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req bookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	b, err := s.kasse.Book(r.Context(), id, kasse.BookingKind(req.Kind), req.AmountCents, req.Purpose, actor(r))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) closeDay(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	day, err := s.kasse.CloseDay(r.Context(), id, actor(r))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, day)
}

// statusFor maps a cash-book error to an HTTP status. Keeping it in one place
// makes the mapping directly testable and keeps the handlers branch-free.
func statusFor(err error) int {
	switch {
	case errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrDayExists), errors.Is(err, kasse.ErrDayClosed):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrAmountNotPositive),
		errors.Is(err, kasse.ErrPurposeRequired),
		errors.Is(err, kasse.ErrUnknownKind):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errors.New("id must be an integer")
	}
	return id, nil
}
