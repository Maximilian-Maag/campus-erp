package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// orderService is the payment-order workflow surface the HTTP layer needs.
type orderService interface {
	CreateRecipient(ctx context.Context, r kasse.Recipient) (kasse.Recipient, error)
	CreateBudgetPosition(ctx context.Context, b kasse.BudgetPosition) (kasse.BudgetPosition, error)
	Create(ctx context.Context, o kasse.PaymentOrder) (kasse.PaymentOrder, error)
	Order(ctx context.Context, id int64) (kasse.PaymentOrder, error)
	Submit(ctx context.Context, id int64, by string) (kasse.PaymentOrder, error)
	Approve(ctx context.Context, id int64, by string) (kasse.PaymentOrder, error)
	Reject(ctx context.Context, id int64, by, reason string) (kasse.PaymentOrder, error)
	Cancel(ctx context.Context, id int64, by string) (kasse.PaymentOrder, error)
}

// statusForOrder maps a payment-order error to an HTTP status. A four-eyes
// violation is a 403: the caller is authenticated but not permitted to approve.
func statusForOrder(err error) int {
	switch {
	case errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrFourEyes):
		return http.StatusForbidden
	case errors.Is(err, kasse.ErrReferenceExists),
		errors.Is(err, kasse.ErrOrderConflict),
		errors.Is(err, kasse.ErrNotDraft),
		errors.Is(err, kasse.ErrNotSubmitted),
		errors.Is(err, kasse.ErrNotCancellable):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrAmountNotPositive),
		errors.Is(err, kasse.ErrPurposeRequired),
		errors.Is(err, kasse.ErrBadCurrency),
		errors.Is(err, kasse.ErrBadYear),
		errors.Is(err, kasse.ErrReferenceRequired),
		errors.Is(err, kasse.ErrRecipientRequired),
		errors.Is(err, kasse.ErrBudgetPositionRequired),
		errors.Is(err, kasse.ErrDueDateRequired),
		errors.Is(err, kasse.ErrReasonRequired),
		errors.Is(err, kasse.ErrAllocationNegative),
		errors.Is(err, budget.ErrBudgetExceeded),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type createOrderRequest struct {
	Reference    string `json:"reference"`
	BusinessYear int    `json:"business_year"`
	AmountCents  int64  `json:"amount_cents"`
	Currency     string `json:"currency"`
	Purpose      string `json:"purpose"`
	DueDate      string `json:"due_date"`
	Recipient    struct {
		Name          string `json:"name"`
		AccountHolder string `json:"account_holder"`
		IBAN          string `json:"iban"`
		BIC           string `json:"bic"`
		BankName      string `json:"bank_name"`
	} `json:"recipient"`
	BudgetPosition struct {
		Year           int    `json:"year"`
		Chapter        string `json:"chapter"`
		Title          string `json:"title"`
		CostCentre     string `json:"cost_centre"`
		CostCarrier    string `json:"cost_carrier"`
		AllocatedCents int64  `json:"allocated_cents"`
	} `json:"budget_position"`
}

func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	due, err := time.Parse(time.DateOnly, req.DueDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "due_date must be YYYY-MM-DD")
		return
	}

	recipient, err := s.orders.CreateRecipient(r.Context(), kasse.Recipient{
		Name:          req.Recipient.Name,
		AccountHolder: req.Recipient.AccountHolder,
		IBAN:          req.Recipient.IBAN,
		BIC:           req.Recipient.BIC,
		BankName:      req.Recipient.BankName,
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}

	position, err := s.orders.CreateBudgetPosition(r.Context(), kasse.BudgetPosition{
		Year:           req.BudgetPosition.Year,
		Chapter:        req.BudgetPosition.Chapter,
		Title:          req.BudgetPosition.Title,
		CostCentre:     req.BudgetPosition.CostCentre,
		CostCarrier:    req.BudgetPosition.CostCarrier,
		AllocatedCents: req.BudgetPosition.AllocatedCents,
	})
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err.Error())
		return
	}

	order, err := kasse.NewPaymentOrder(req.Reference, req.BusinessYear, req.AmountCents, req.Currency,
		recipient.ID, position.ID, req.Purpose, due, actor(r))
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	saved, err := s.orders.Create(r.Context(), order)
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := s.orders.Order(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) submitOrder(w http.ResponseWriter, r *http.Request) {
	s.orderTransition(w, r, func(id int64, by string) (kasse.PaymentOrder, error) {
		return s.orders.Submit(r.Context(), id, by)
	})
}

func (s *Server) approveOrder(w http.ResponseWriter, r *http.Request) {
	s.orderTransition(w, r, func(id int64, by string) (kasse.PaymentOrder, error) {
		return s.orders.Approve(r.Context(), id, by)
	})
}

func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) {
	s.orderTransition(w, r, func(id int64, by string) (kasse.PaymentOrder, error) {
		return s.orders.Cancel(r.Context(), id, by)
	})
}

func (s *Server) rejectOrder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	order, err := s.orders.Reject(r.Context(), id, actor(r), req.Reason)
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) orderTransition(w http.ResponseWriter, r *http.Request,
	fn func(int64, string) (kasse.PaymentOrder, error)) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := fn(id, actor(r))
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}
