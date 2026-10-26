package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// budgetService is the budget surface the HTTP layer needs.
type budgetService interface {
	CreatePosition(ctx context.Context, year int, chapter, title, description string, allocatedCents int64, by string) (budget.Position, error)
	Post(ctx context.Context, positionID int64, kind budget.PostingKind, amountCents int64, reference, by string) (budget.Posting, error)
	GetPosition(ctx context.Context, id int64) (budget.Position, error)
	Balance(ctx context.Context, id int64) (budget.Balance, error)
}

// statusForBudget maps a budget error to an HTTP status.
func statusForBudget(err error) int {
	switch {
	case errors.Is(err, budget.ErrPositionNotFound):
		return http.StatusNotFound
	case errors.Is(err, budget.ErrPositionExists), errors.Is(err, budget.ErrBudgetExceeded):
		return http.StatusConflict
	case errors.Is(err, budget.ErrActorRequired),
		errors.Is(err, budget.ErrYearRequired),
		errors.Is(err, budget.ErrChapterRequired),
		errors.Is(err, budget.ErrTitleRequired),
		errors.Is(err, budget.ErrAllocatedNegative),
		errors.Is(err, budget.ErrAmountNotPositive),
		errors.Is(err, budget.ErrUnknownKind):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type createBudgetPositionRequest struct {
	Year           int    `json:"year"`
	Chapter        string `json:"chapter"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	AllocatedCents int64  `json:"allocated_cents"`
}

func (s *Server) createBudgetPosition(w http.ResponseWriter, r *http.Request) {
	var req createBudgetPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pos, err := s.budget.CreatePosition(r.Context(), req.Year, req.Chapter, req.Title, req.Description, req.AllocatedCents, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForBudget(err))
		return
	}
	writeJSON(w, http.StatusCreated, pos)
}

type budgetPositionResponse struct {
	Position budget.Position `json:"position"`
	Balance  budget.Balance  `json:"balance"`
}

func (s *Server) getBudgetPosition(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pos, err := s.budget.GetPosition(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForBudget(err))
		return
	}
	bal, err := s.budget.Balance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForBudget(err))
		return
	}
	writeJSON(w, http.StatusOK, budgetPositionResponse{Position: pos, Balance: bal})
}

type budgetPostingRequest struct {
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Reference   string `json:"reference"`
}

func (s *Server) postBudget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req budgetPostingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	posting, err := s.budget.Post(r.Context(), id, budget.PostingKind(req.Kind), req.AmountCents, req.Reference, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForBudget(err))
		return
	}
	writeJSON(w, http.StatusCreated, posting)
}

func (s *Server) getBudgetBalance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bal, err := s.budget.Balance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForBudget(err))
		return
	}
	writeJSON(w, http.StatusOK, bal)
}
