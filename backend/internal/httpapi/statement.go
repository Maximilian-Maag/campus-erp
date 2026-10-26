package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// statementService is the bank-statement surface the HTTP layer needs.
type statementService interface {
	Import(ctx context.Context, accountID int64, content, by string) ([]kasse.Statement, error)
	Statement(ctx context.Context, id int64) (kasse.Statement, error)
}

// statusForStatement maps a statement error to an HTTP status.
func statusForStatement(err error) int {
	switch {
	case errors.Is(err, kasse.ErrStatementNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrStatementEmpty),
		errors.Is(err, kasse.ErrStatementInvalid),
		errors.Is(err, kasse.ErrCashAccountRequired),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type importStatementRequest struct {
	AccountID int64  `json:"account_id"`
	Content   string `json:"content"`
}

func (s *Server) importStatement(w http.ResponseWriter, r *http.Request) {
	var req importStatementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	stmts, err := s.statements.Import(r.Context(), req.AccountID, req.Content, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForStatement(err))
		return
	}
	writeJSON(w, http.StatusCreated, stmts)
}

func (s *Server) getStatement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	st, err := s.statements.Statement(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForStatement(err))
		return
	}
	writeJSON(w, http.StatusOK, st)
}
