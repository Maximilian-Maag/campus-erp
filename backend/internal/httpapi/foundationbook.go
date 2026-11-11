package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// foundationBookService is the foundation-book surface the HTTP layer needs.
type foundationBookService interface {
	CreateBook(ctx context.Context, foundationID int64, reference, currency, by string) (kasse.FoundationBook, error)
	Book(ctx context.Context, bookID, amountCents int64, kind kasse.BookingKind, purpose, by string) (kasse.FoundationBooking, error)
	Bookings(ctx context.Context, bookID int64) ([]kasse.FoundationBooking, error)
	Books(ctx context.Context, foundationID int64) ([]kasse.FoundationBook, error)
	Balance(ctx context.Context, bookID int64) (kasse.FoundationBalance, error)
}

// statusForFoundationBook maps a foundation-book error to an HTTP status.
func statusForFoundationBook(err error) int {
	switch {
	case errors.Is(err, kasse.ErrFoundationBookNotFound), errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrFoundationBookExists):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrFoundationBookReferenceRequired),
		errors.Is(err, kasse.ErrFoundationRequired),
		errors.Is(err, kasse.ErrAmountNotPositive),
		errors.Is(err, kasse.ErrPurposeRequired),
		errors.Is(err, kasse.ErrUnknownKind),
		errors.Is(err, kasse.ErrBadCurrency),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type createFoundationBookRequest struct {
	FoundationID int64  `json:"foundation_id"`
	Reference    string `json:"reference"`
	Currency     string `json:"currency"`
}

func (s *Server) createFoundationBook(w http.ResponseWriter, r *http.Request) {
	var req createFoundationBookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	book, err := s.foundationBooks.CreateBook(r.Context(), req.FoundationID, req.Reference, req.Currency, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForFoundationBook(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, book)
}

type bookFoundationRequest struct {
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Purpose     string `json:"purpose"`
}

func (s *Server) bookFoundation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req bookFoundationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	booking, err := s.foundationBooks.Book(r.Context(), id, req.AmountCents, kasse.BookingKind(req.Kind), req.Purpose, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForFoundationBook(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, booking)
}

func (s *Server) getFoundationBookings(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	bookings, err := s.foundationBooks.Bookings(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForFoundationBook(err), err)
		return
	}
	writeJSON(w, http.StatusOK, bookings)
}

func (s *Server) getFoundationBalance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	bal, err := s.foundationBooks.Balance(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForFoundationBook(err), err)
		return
	}
	writeJSON(w, http.StatusOK, bal)
}

// listFoundationBooks reads one foundation's books. The foundation is a path segment rather
// than a query, so a request cannot be read as asking for another foundation's books.
func (s *Server) listFoundationBooks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	books, err := s.foundationBooks.Books(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForFoundationBook(err), err)
		return
	}
	writeJSON(w, http.StatusOK, books)
}
