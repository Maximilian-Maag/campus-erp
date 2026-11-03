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
	Positions(ctx context.Context) ([]budget.Position, error)
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
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	pos, err := s.budget.CreatePosition(r.Context(), req.Year, req.Chapter, req.Title, req.Description, req.AllocatedCents, actor(r))
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pos)
}

// budgetPositionResponse is a position with its balance and the amount still free. The
// available amount is derived here, from the same balance the Mittelüberwachung guard uses, so
// a client shows what the budget will allow without re-implementing the rule.
type budgetPositionResponse struct {
	Position       budget.Position `json:"position"`
	Balance        budget.Balance  `json:"balance"`
	AvailableCents int64           `json:"available_cents"`
}

func (s *Server) getBudgetPosition(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	pos, err := s.budget.GetPosition(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	bal, err := s.budget.Balance(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, budgetPositionResponse{Position: pos, Balance: bal, AvailableCents: bal.AvailableCents()})
}

// listBudgetPositions returns every position with its balance: the Mittelübersicht a client
// shows without having to ask position by position. It reads each balance separately, so a
// budget with many positions costs many reads; an aggregate query is the follow-up when it
// matters.
func (s *Server) listBudgetPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := s.budget.Positions(r.Context())
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	out := make([]budgetPositionResponse, 0, len(positions))
	for _, pos := range positions {
		bal, err := s.budget.Balance(r.Context(), pos.ID)
		if err != nil {
			writeProblem(w, statusForBudget(err), err.Error())
			return
		}
		out = append(out, budgetPositionResponse{Position: pos, Balance: bal, AvailableCents: bal.AvailableCents()})
	}
	writeJSON(w, http.StatusOK, out)
}

type budgetPostingRequest struct {
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Reference   string `json:"reference"`
}

func (s *Server) postBudget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req budgetPostingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	posting, err := s.budget.Post(r.Context(), id, budget.PostingKind(req.Kind), req.AmountCents, req.Reference, actor(r))
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, posting)
}

func (s *Server) getBudgetBalance(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	bal, err := s.budget.Balance(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bal)
}
