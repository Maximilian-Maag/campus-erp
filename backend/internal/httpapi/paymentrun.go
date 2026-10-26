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

type createCashAccountRequest struct {
	Name     string `json:"name"`
	IBAN     string `json:"iban"`
	BIC      string `json:"bic"`
	Currency string `json:"currency"`
}

func (s *Server) createCashAccount(w http.ResponseWriter, r *http.Request) {
	var req createCashAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	account, err := s.runs.CreateCashAccount(r.Context(), req.Name, req.IBAN, req.BIC, req.Currency)
	if err != nil {
		http.Error(w, err.Error(), statusForRun(err))
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	executionDate, err := time.Parse(time.DateOnly, req.ExecutionDate)
	if err != nil {
		http.Error(w, "execution_date must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	run, err := s.runs.CreateRun(r.Context(), req.Reference, req.AccountID, executionDate, req.OrderIDs, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForRun(err))
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	run, err := s.runs.Run(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForRun(err))
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) getRunFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, err := s.runs.RunFile(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), statusForRun(err))
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(file))
}
