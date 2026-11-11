package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// statementService is the bank-statement surface the HTTP layer needs.
type statementService interface {
	Import(ctx context.Context, accountID int64, content, by string) ([]kasse.Statement, error)
	Statement(ctx context.Context, id int64) (kasse.Statement, error)
	Statements(ctx context.Context, accountID int64) ([]kasse.StatementSummary, error)
	BankFile(ctx context.Context, id int64) (kasse.BankFile, error)
}

// statusForStatement maps a statement error to an HTTP status.
func statusForStatement(err error) int {
	switch {
	case errors.Is(err, kasse.ErrStatementNotFound), errors.Is(err, kasse.ErrBankFileNotFound):
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
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	stmts, err := s.statements.Import(r.Context(), req.AccountID, req.Content, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForStatement(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, stmts)
}

// listStatements reads the imported statements, optionally narrowed to one account. The filter
// is the server's, so the screen never has to hold every statement to show one account's.
func (s *Server) listStatements(w http.ResponseWriter, r *http.Request) {
	accountID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("account_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_account", "account_id must be a positive number")
			return
		}
		accountID = parsed
	}
	stmts, err := s.statements.Statements(r.Context(), accountID)
	if err != nil {
		writeDomainProblem(w, statusForStatement(err), err)
		return
	}
	writeJSON(w, http.StatusOK, stmts)
}

func (s *Server) getStatement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := s.statements.Statement(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForStatement(err), err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) getBankFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	file, err := s.statements.BankFile(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForStatement(err), err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}
