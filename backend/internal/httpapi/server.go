// Package httpapi exposes the backend's HTTP surface. Handlers stay thin: they
// decode, delegate to a domain service, and map errors to status codes.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
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

// Server wires HTTP handlers to the domain services.
type Server struct {
	kasse   cashBook
	orders  orderService
	dunning dunningService
	audit   auditVerifier
}

// New returns a Server.
func New(k cashBook, o orderService, d dunningService, a auditVerifier) *Server {
	return &Server{kasse: k, orders: o, dunning: d, audit: a}
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
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

	mux.HandleFunc("POST /api/dunning/levels", s.createDunningLevel)
	mux.HandleFunc("GET /api/dunning/levels/{year}/{code}", s.getDunningLevel)
	mux.HandleFunc("POST /api/dunning/notices", s.issueDunningNotice)
	return mux
}

// actor identifies the caller until real auth lands (OIDC, ADR-0003).
func actor(r *http.Request) string {
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

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) auditVerify(w http.ResponseWriter, r *http.Request) {
	if err := s.audit.Verify(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", req.BusinessDate)
	if err != nil {
		http.Error(w, "business_date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	day, err := s.kasse.OpenDay(r.Context(), date, req.OpeningCents, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	writeJSON(w, http.StatusCreated, day)
}

func (s *Server) getDay(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	day, err := s.kasse.Day(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req bookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	b, err := s.kasse.Book(r.Context(), id, kasse.BookingKind(req.Kind), req.AmountCents, req.Purpose, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) closeDay(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	day, err := s.kasse.CloseDay(r.Context(), id, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
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
