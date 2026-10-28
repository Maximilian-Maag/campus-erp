package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// procurementService is the purchase-order surface the HTTP layer needs.
type procurementService interface {
	Create(ctx context.Context, number, supplier string, budgetPositionID int64, currency string, lines []procurement.Line, by string) (procurement.Order, error)
	Approve(ctx context.Context, id int64, by string) (procurement.Order, error)
	Receive(ctx context.Context, id int64, by string) (procurement.Order, error)
	Cancel(ctx context.Context, id int64, by string) (procurement.Order, error)
	Get(ctx context.Context, id int64) (procurement.Order, error)
}

// statusForProcurement maps a procurement error to an HTTP status.
func statusForProcurement(err error) int {
	switch {
	case errors.Is(err, procurement.ErrOrderNotFound):
		return http.StatusNotFound
	case errors.Is(err, procurement.ErrOrderExists),
		errors.Is(err, procurement.ErrBadTransition),
		errors.Is(err, budget.ErrBudgetExceeded):
		return http.StatusConflict
	case errors.Is(err, procurement.ErrActorRequired),
		errors.Is(err, procurement.ErrNumberRequired),
		errors.Is(err, procurement.ErrSupplierRequired),
		errors.Is(err, procurement.ErrBudgetPositionRequired),
		errors.Is(err, procurement.ErrCurrency),
		errors.Is(err, procurement.ErrNoLines),
		errors.Is(err, procurement.ErrLineDescription),
		errors.Is(err, procurement.ErrLineQuantity),
		errors.Is(err, procurement.ErrLineAmount),
		errors.Is(err, procurement.ErrVATCategoryRequired),
		errors.Is(err, procurement.ErrLineNetMismatch):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type purchaseOrderLineRequest struct {
	LineNo         int    `json:"line_no"`
	Description    string `json:"description"`
	QuantityMilli  int64  `json:"quantity_milli"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	NetCents       int64  `json:"net_cents"`
	VatCategory    string `json:"vat_category"`
	VatRateBP      int64  `json:"vat_rate_bp"`
}

type createPurchaseOrderRequest struct {
	Number           string                     `json:"number"`
	Supplier         string                     `json:"supplier"`
	BudgetPositionID int64                      `json:"budget_position_id"`
	Currency         string                     `json:"currency"`
	Lines            []purchaseOrderLineRequest `json:"lines"`
}

func (s *Server) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var req createPurchaseOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	lines := make([]procurement.Line, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, procurement.Line{
			LineNo: l.LineNo, Description: l.Description, QuantityMilli: l.QuantityMilli,
			UnitPriceCents: l.UnitPriceCents, NetCents: l.NetCents,
			VatCategory: l.VatCategory, VatRateBP: l.VatRateBP,
		})
	}
	order, err := s.procurement.Create(r.Context(), req.Number, req.Supplier, req.BudgetPositionID, req.Currency, lines, actor(r))
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := s.procurement.Get(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) approvePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.purchaseOrderStep(w, r, s.procurement.Approve)
}

func (s *Server) receivePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.purchaseOrderStep(w, r, s.procurement.Receive)
}

func (s *Server) cancelPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	s.purchaseOrderStep(w, r, s.procurement.Cancel)
}

func (s *Server) purchaseOrderStep(w http.ResponseWriter, r *http.Request, step func(context.Context, int64, string) (procurement.Order, error)) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := step(r.Context(), id, actor(r))
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}
