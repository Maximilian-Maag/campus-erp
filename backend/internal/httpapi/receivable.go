package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// receivableService is the open-items surface the HTTP layer needs.
type receivableService interface {
	Post(ctx context.Context, mandateReference string, amountCents int64, currency string,
		dueDate time.Time, purpose, source, by string) (kasse.Receivable, error)
	Receivable(ctx context.Context, id int64) (kasse.Receivable, error)
	Receivables(ctx context.Context, state kasse.ReceivableState) ([]kasse.Receivable, error)
}

// statusForReceivable maps a receivable error to an HTTP status.
func statusForReceivable(err error) int {
	switch {
	case errors.Is(err, kasse.ErrReceivableNotFound), errors.Is(err, kasse.ErrMandateNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrMandateReferenceRequired),
		errors.Is(err, kasse.ErrMandateReferenceTooLong),
		errors.Is(err, kasse.ErrReceivableAmountNotPositive),
		errors.Is(err, kasse.ErrReceivableCurrencyRequired),
		errors.Is(err, kasse.ErrReceivableDueDateRequired),
		errors.Is(err, kasse.ErrReceivablePurposeRequired),
		errors.Is(err, kasse.ErrReceivablePurposeTooLong),
		errors.Is(err, kasse.ErrReceivableNotOpen),
		errors.Is(err, kasse.ErrReceivableCurrencyMismatch),
		errors.Is(err, kasse.ErrReceivableDueByRequired),
		errors.Is(err, kasse.ErrReceivableDueByAfterCollection),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type receivableRequest struct {
	MandateReference string `json:"mandate_reference"`
	AmountCents      int64  `json:"amount_cents"`
	Currency         string `json:"currency"`
	DueDate          string `json:"due_date"` // YYYY-MM-DD
	Purpose          string `json:"purpose"`
	Source           string `json:"source"`
}

func (s *Server) createReceivable(w http.ResponseWriter, r *http.Request) {
	var req receivableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	dueDate, err := time.Parse(time.DateOnly, req.DueDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
		return
	}
	saved, err := s.receivables.Post(r.Context(), req.MandateReference, req.AmountCents, req.Currency,
		dueDate, req.Purpose, req.Source, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForReceivable(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// listReceivables reads the open items, due date first, optionally narrowed to one
// state with ?state=. The filter is the server's, so a screen never has to hold
// every item to show the open ones.
func (s *Server) listReceivables(w http.ResponseWriter, r *http.Request) {
	state := kasse.ReceivableState(strings.TrimSpace(r.URL.Query().Get("state")))
	switch state {
	case "", kasse.ReceivableOpen, kasse.ReceivableCollected, kasse.ReceivableCancelled:
	default:
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_state", "state must be open, collected or cancelled")
		return
	}
	items, err := s.receivables.Receivables(r.Context(), state)
	if err != nil {
		writeDomainProblem(w, statusForReceivable(err), err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getReceivable(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "id must be a number")
		return
	}
	item, err := s.receivables.Receivable(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForReceivable(err), err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
