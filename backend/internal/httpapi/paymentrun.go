package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// paymentRunService is the payment-run surface the HTTP layer needs.
type paymentRunService interface {
	CreateCashAccount(ctx context.Context, name, iban, bic, currency string) (kasse.CashAccount, error)
	CashAccounts(ctx context.Context) ([]kasse.CashAccount, error)
	CreateRun(ctx context.Context, reference string, accountID int64, executionDate time.Time, orderIDs []int64, by string) (kasse.PaymentRun, error)
	Run(ctx context.Context, id int64) (kasse.PaymentRun, error)
	RunFile(ctx context.Context, id int64) (string, error)
}

// statusForRun maps a payment-run error to an HTTP status.
func statusForRun(err error) int {
	switch {
	case errors.Is(err, kasse.ErrRunNotFound), errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrRunReferenceExists), errors.Is(err, kasse.ErrOrderAlreadyPaid):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrRunReferenceRequired),
		errors.Is(err, kasse.ErrNoOrders),
		errors.Is(err, kasse.ErrOrderNotApproved),
		errors.Is(err, kasse.ErrCashAccountRequired),
		errors.Is(err, kasse.ErrExecutionDateRequired),
		errors.Is(err, kasse.ErrIBANInvalid),
		errors.Is(err, kasse.ErrBICInvalid),
		errors.Is(err, kasse.ErrBadCurrency),
		errors.Is(err, kasse.ErrRecipientRequired),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// listCashAccounts reads the university's own accounts, which a screen has to offer before it can
// ask about one.
func (s *Server) listCashAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.runs.CashAccounts(r.Context())
	if err != nil {
		writeProblem(w, statusForRun(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

type createCashAccountRequest struct {
	Name     string `json:"name"`
	IBAN     string `json:"iban"`
	BIC      string `json:"bic"`
	Currency string `json:"currency"`
}

func (s *Server) createCashAccount(w http.ResponseWriter, r *http.Request) {
	var req createCashAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	account, err := s.runs.CreateCashAccount(r.Context(), req.Name, req.IBAN, req.BIC, req.Currency)
	if err != nil {
		writeProblem(w, statusForRun(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

type createRunRequest struct {
	Reference     string  `json:"reference"`
	AccountID     int64   `json:"account_id"`
	ExecutionDate string  `json:"execution_date"`
	OrderIDs      []int64 `json:"order_ids"`
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	executionDate, err := time.Parse(time.DateOnly, req.ExecutionDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "execution_date must be YYYY-MM-DD")
		return
	}
	run, err := s.runs.CreateRun(r.Context(), req.Reference, req.AccountID, executionDate, req.OrderIDs, actor(r))
	if err != nil {
		writeProblem(w, statusForRun(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.runs.Run(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForRun(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) getRunFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	file, err := s.runs.RunFile(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForRun(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(file))
}
