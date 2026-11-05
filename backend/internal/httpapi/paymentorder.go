package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	Orders(ctx context.Context, status kasse.OrderStatus) ([]kasse.OrderSummary, error)
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

// recipientRequest and budgetPositionRequest are the nested bodies of a new order. They
// are named types rather than anonymous structs so the contract generator keeps them
// nested: an anonymous struct is flattened into its parent, which documents a body the
// route does not accept.
type recipientRequest struct {
	Name          string `json:"name"`
	AccountHolder string `json:"account_holder"`
	IBAN          string `json:"iban"`
	BIC           string `json:"bic"`
	BankName      string `json:"bank_name"`
}

type budgetPositionRequest struct {
	// ID names an existing position — one created through the budget module — instead of
	// describing a new one. It is the one field that is not part of a description.
	ID             int64  `json:"id,omitempty"`
	Year           int    `json:"year"`
	Chapter        string `json:"chapter"`
	Title          string `json:"title"`
	CostCentre     string `json:"cost_centre"`
	CostCarrier    string `json:"cost_carrier"`
	AllocatedCents int64  `json:"allocated_cents"`
}

// describesAPosition reports whether the request carries the fields of a new position.
// An allocation of zero on its own is not a description: a caller that only names an id
// leaves every other field at its zero value.
func (b budgetPositionRequest) describesAPosition() bool {
	return b.Year != 0 || b.Chapter != "" || b.Title != "" || b.CostCentre != "" ||
		b.CostCarrier != "" || b.AllocatedCents != 0
}

// errBudgetPositionAmbiguous is returned when an order both names an existing budget
// position and describes a new one. The two are alternatives, and silently honouring one
// would ignore what the caller wrote — so it is refused instead.
var errBudgetPositionAmbiguous = errors.New("name an existing budget position or describe a new one, not both")

// budgetPositionForOrder resolves the position an order draws on: an existing one by id,
// or a new one described inline. Naming an existing position does not rewrite its Ansatz
// (docs/10), so the caller names one or describes one — never both.
func (s *Server) budgetPositionForOrder(ctx context.Context, req budgetPositionRequest) (int64, error) {
	if req.ID > 0 {
		if req.describesAPosition() {
			return 0, errBudgetPositionAmbiguous
		}
		existing, err := s.budget.GetPosition(ctx, req.ID)
		if err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	created, err := s.orders.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year:           req.Year,
		Chapter:        req.Chapter,
		Title:          req.Title,
		CostCentre:     req.CostCentre,
		CostCarrier:    req.CostCarrier,
		AllocatedCents: req.AllocatedCents,
	})
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

type createOrderRequest struct {
	Reference      string                `json:"reference"`
	BusinessYear   int                   `json:"business_year"`
	AmountCents    int64                 `json:"amount_cents"`
	Currency       string                `json:"currency"`
	Purpose        string                `json:"purpose"`
	DueDate        string                `json:"due_date"`
	Recipient      recipientRequest      `json:"recipient"`
	BudgetPosition budgetPositionRequest `json:"budget_position"`
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

	positionID, err := s.budgetPositionForOrder(r.Context(), req.BudgetPosition)
	if err != nil {
		if errors.Is(err, errBudgetPositionAmbiguous) {
			writeProblemCode(w, http.StatusUnprocessableEntity, "budget_position_ambiguous", err.Error())
			return
		}
		// A position that is not there is the budget module's to report, and it is a client
		// error, not a server one.
		writeProblem(w, statusForBudget(err), err.Error())
		return
	}

	order, err := kasse.NewPaymentOrder(req.Reference, req.BusinessYear, req.AmountCents, req.Currency,
		recipient.ID, positionID, req.Purpose, due, actor(r))
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

// listOrders reads the payment orders, optionally narrowed to one status. The filter is the
// server's, so the screen never has to hold the whole list to work on part of it.
func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := s.orders.Orders(r.Context(), kasse.OrderStatus(strings.TrimSpace(r.URL.Query().Get("status"))))
	if err != nil {
		writeProblem(w, statusForOrder(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orders)
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
