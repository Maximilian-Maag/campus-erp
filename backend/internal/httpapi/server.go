// Package httpapi exposes the backend's HTTP surface. Handlers stay thin: they
// decode, delegate to a domain service, and map errors to status codes.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/auth"
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
	audit           auditVerifier
	health          healthChecker
	auth            auth.Verifier
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
	Audit           auditVerifier
	Health          healthChecker
	Auth            auth.Verifier
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
		audit:           d.Audit,
		health:          d.Health,
		auth:            d.Auth,
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
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /api/audit/verify", s.auditVerify)
	mux.HandleFunc("POST /api/cashbook/days", s.openDay)
	mux.HandleFunc("GET /api/cashbook/days/{id}", s.getDay)
	mux.HandleFunc("POST /api/cashbook/days/{id}/bookings", s.book)
	mux.HandleFunc("POST /api/cashbook/days/{id}/close", s.closeDay)

	mux.HandleFunc("POST /api/payment-orders", s.createOrder)
	mux.HandleFunc("GET /api/payment-orders/{id}", s.getOrder)
	mux.HandleFunc("POST /api/payment-orders/{id}/submit", s.submitOrder)
	mux.HandleFunc("POST /api/payment-orders/{id}/approve", s.approveOrder)
	mux.HandleFunc("POST /api/payment-orders/{id}/reject", s.rejectOrder)
	mux.HandleFunc("POST /api/payment-orders/{id}/cancel", s.cancelOrder)

	mux.HandleFunc("POST /api/cash-accounts", s.createCashAccount)
	mux.HandleFunc("POST /api/payment-runs", s.createRun)
	mux.HandleFunc("GET /api/payment-runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/payment-runs/{id}/file", s.getRunFile)

	mux.HandleFunc("POST /api/bank-statements", s.importStatement)
	mux.HandleFunc("GET /api/bank-statements/{id}", s.getStatement)
	mux.HandleFunc("GET /api/bank-files/{id}", s.getBankFile)

	mux.HandleFunc("POST /api/foundation-books", s.createFoundationBook)
	mux.HandleFunc("POST /api/foundation-books/{id}/bookings", s.bookFoundation)
	mux.HandleFunc("GET /api/foundation-books/{id}/bookings", s.getFoundationBookings)
	mux.HandleFunc("GET /api/foundation-books/{id}/balance", s.getFoundationBalance)

	mux.HandleFunc("POST /api/e-invoices", s.createEInvoice)
	mux.HandleFunc("POST /api/e-invoices/received", s.receiveEInvoice)
	mux.HandleFunc("GET /api/e-invoices/{id}", s.getEInvoice)
	mux.HandleFunc("GET /api/e-invoices/{id}/xrechnung", s.getEInvoiceXML)

	mux.HandleFunc("POST /api/budget/positions", s.createBudgetPosition)
	mux.HandleFunc("GET /api/budget/positions/{id}", s.getBudgetPosition)
	mux.HandleFunc("POST /api/budget/positions/{id}/postings", s.postBudget)
	mux.HandleFunc("GET /api/budget/positions/{id}/balance", s.getBudgetBalance)

	mux.HandleFunc("POST /api/lookups/seed", s.seedLookups)
	mux.HandleFunc("GET /api/lookups/{namespace}", s.listLookups)
	mux.HandleFunc("GET /api/lookups/{namespace}/{code}", s.getLookup)
	mux.HandleFunc("POST /api/lookups/{namespace}", s.upsertLookups)

	mux.HandleFunc("POST /api/purchase-orders", s.createPurchaseOrder)
	mux.HandleFunc("GET /api/purchase-orders/{id}", s.getPurchaseOrder)
	mux.HandleFunc("POST /api/purchase-orders/{id}/approve", s.approvePurchaseOrder)
	mux.HandleFunc("POST /api/purchase-orders/{id}/receive", s.receivePurchaseOrder)
	mux.HandleFunc("POST /api/purchase-orders/{id}/cancel", s.cancelPurchaseOrder)

	mux.HandleFunc("POST /api/klr/cost-centers", s.createCostCenter)
	mux.HandleFunc("POST /api/klr/cost-units", s.createCostUnit)
	mux.HandleFunc("POST /api/klr/cost-centers/{id}/records", s.postAccountingRecord)
	mux.HandleFunc("GET /api/klr/cost-centers/{id}/report", s.getKLRReport)

	mux.HandleFunc("POST /api/dunning/levels", s.createDunningLevel)
	mux.HandleFunc("GET /api/dunning/levels/{year}/{code}", s.getDunningLevel)
	mux.HandleFunc("POST /api/dunning/notices", s.issueDunningNotice)

	mux.HandleFunc("POST /api/vat-types", s.createVATType)
	mux.HandleFunc("GET /api/vat-types/{code}", s.getVATType)
	mux.HandleFunc("POST /api/tax-offices", s.createTaxOffice)
	mux.HandleFunc("POST /api/foundations", s.createFoundation)
	// Auth is part of the route table; correlation id, recovery and the access
	// log are added once by Handler (or by the Tenancy front) (NFR-5, ADR-0003).
	return s.authenticate(mux)
}

// actor is the caller recorded on audit events: the authenticated subject when a
// token was presented, otherwise the dev X-Actor header, otherwise "anonymous".
func actor(r *http.Request) string {
	if p, ok := principalFrom(r.Context()); ok && p.Subject != "" {
		return p.Subject
	}
	if a := r.Header.Get("X-Actor"); a != "" {
		return a
	}
	return "anonymous"
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
