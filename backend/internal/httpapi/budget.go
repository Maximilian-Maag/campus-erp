package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
)

// budgetService is the budget surface the HTTP layer needs.
type budgetService interface {
	CreatePosition(ctx context.Context, year int, chapter, title, description string, allocatedCents int64, by string) (budget.Position, error)
	Post(ctx context.Context, positionID int64, kind budget.PostingKind, amountCents int64, reference, by string) (budget.Posting, error)
	GetPosition(ctx context.Context, id int64) (budget.Position, error)
	Positions(ctx context.Context, limit, offset int) ([]budget.Position, error)
	Balance(ctx context.Context, id int64) (budget.Balance, error)
	// Import reads a budget file — Mittelquellen, Bewirtschaftungsstellen and Haushaltsstellen in
	// one pass — and writes it (docs/10).
	Import(ctx context.Context, layoutID, content, by string) (budget.ImportResult, error)
	// The Mittelquellen and Bewirtschaftungsstellen a position is assigned to (docs/10).
	CreateSource(ctx context.Context, code, name, chapter, by string) (budget.Source, error)
	Sources(ctx context.Context) ([]budget.Source, error)
	CreateCentre(ctx context.Context, code, name, by string) (budget.Centre, error)
	Centres(ctx context.Context) ([]budget.Centre, error)
	AssignPosition(ctx context.Context, positionID int64, sourceID, centreID *int64, by string) (budget.Position, error)
}

// budgetImportRequest is the payload of POST /api/budget/imports. The layout is optional: an
// operator who does not name one gets the standard layout, and a layout this build cannot read is
// refused rather than guessed at.
type budgetImportRequest struct {
	Layout  string `json:"layout"`
	Content string `json:"content"`
}

// importBudget reads a budget file and writes it. A file with an unreadable line is refused whole
// (422, naming the line), so a refusal never leaves half a budget behind.
func (s *Server) importBudget(w http.ResponseWriter, r *http.Request) {
	var req budgetImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	res, err := s.budget.Import(r.Context(), req.Layout, req.Content, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// statusForBudget maps a budget error to an HTTP status.
func statusForBudget(err error) int {
	switch {
	case errors.Is(err, budget.ErrPositionNotFound),
		errors.Is(err, budget.ErrSourceNotFound),
		errors.Is(err, budget.ErrCentreNotFound):
		return http.StatusNotFound
	case errors.Is(err, budgetimport.ErrUnknownLayout),
		errors.Is(err, budgetimport.ErrEmpty),
		errors.Is(err, budgetimport.ErrBadRow):
		return http.StatusUnprocessableEntity
	case errors.Is(err, budget.ErrPositionExists),
		errors.Is(err, budget.ErrSourceExists),
		errors.Is(err, budget.ErrCentreExists),
		errors.Is(err, budget.ErrBudgetExceeded):
		return http.StatusConflict
	case errors.Is(err, budget.ErrActorRequired),
		errors.Is(err, budget.ErrYearRequired),
		errors.Is(err, budget.ErrChapterRequired),
		errors.Is(err, budget.ErrTitleRequired),
		errors.Is(err, budget.ErrAllocatedNegative),
		errors.Is(err, budget.ErrAmountNotPositive),
		errors.Is(err, budget.ErrUnknownKind),
		errors.Is(err, budget.ErrSourceCodeRequired),
		errors.Is(err, budget.ErrSourceNameRequired),
		errors.Is(err, budget.ErrCentreCodeRequired),
		errors.Is(err, budget.ErrCentreNameRequired),
		// An assignment naming a source or a centre nothing carries is a problem with the
		// request rather than with the position: the position itself was found.
		errors.Is(err, budget.ErrAssignmentUnknown):
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
		writeDomainProblem(w, statusForBudget(err), err)
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
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	bal, err := s.budget.Balance(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusOK, budgetPositionResponse{Position: pos, Balance: bal, AvailableCents: bal.AvailableCents()})
}

// listBudgetPositions returns the positions with their balances, one page at a time: the
// Mittelübersicht a client shows without having to ask position by position. The page is what makes
// the per-position balance read below affordable — it is a read per row of the page, not per row of
// the budget; an aggregate query is still the follow-up when it matters.
func (s *Server) listBudgetPositions(w http.ResponseWriter, r *http.Request) {
	limit, offset, problem := pageParams(r)
	if problem != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, problem.Code, problem.Detail)
		return
	}
	positions, err := s.budget.Positions(r.Context(), limit, offset)
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	out := make([]budgetPositionResponse, 0, len(positions))
	for _, pos := range positions {
		bal, err := s.budget.Balance(r.Context(), pos.ID)
		if err != nil {
			writeDomainProblem(w, statusForBudget(err), err)
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
		writeDomainProblem(w, statusForBudget(err), err)
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
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusOK, bal)
}

// budgetSourceRequest is the payload of POST /api/budget/sources.
type budgetSourceRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Chapter string `json:"chapter"`
}

// budgetCentreRequest is the payload of POST /api/budget/centres.
type budgetCentreRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// assignBudgetPositionRequest is the payload of POST /api/budget/positions/{id}/assignment. Both
// halves are nullable: naming one and clearing the other is one operation, and omitting a field
// (or sending null) clears it.
type assignBudgetPositionRequest struct {
	SourceID *int64 `json:"source_id"`
	CentreID *int64 `json:"centre_id"`
}

func (s *Server) createBudgetSource(w http.ResponseWriter, r *http.Request) {
	var req budgetSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	src, err := s.budget.CreateSource(r.Context(), req.Code, req.Name, req.Chapter, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

// listBudgetSources returns the Mittelquellen in code order.
func (s *Server) listBudgetSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.budget.Sources(r.Context())
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) createBudgetCentre(w http.ResponseWriter, r *http.Request) {
	var req budgetCentreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	centre, err := s.budget.CreateCentre(r.Context(), req.Code, req.Name, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, centre)
}

// listBudgetCentres returns the Bewirtschaftungsstellen in code order.
func (s *Server) listBudgetCentres(w http.ResponseWriter, r *http.Request) {
	centres, err := s.budget.Centres(r.Context())
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusOK, centres)
}

// assignBudgetPosition names the source a position draws on and the centre that administers it.
func (s *Server) assignBudgetPosition(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req assignBudgetPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	pos, err := s.budget.AssignPosition(r.Context(), id, req.SourceID, req.CentreID, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForBudget(err), err)
		return
	}
	writeJSON(w, http.StatusOK, pos)
}
