package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
	Orders(ctx context.Context, status procurement.Status) ([]procurement.OrderSummary, error)
	// RecordReceipt records a delivery (a goods receipt) against an approved order (docs/12).
	RecordReceipt(ctx context.Context, orderID int64, reference string, receivedOn time.Time, lines []procurement.ReceiptLine, by string) (procurement.ReceiptResult, error)
	// ReceiptState is the order's goods-receipt state: every line's progress and the receipts.
	ReceiptState(ctx context.Context, orderID int64) (procurement.ReceiptState, error)
}

// listPurchaseOrders returns the order summaries, optionally narrowed to one status. An
// unknown status is refused rather than quietly listing every order, and an empty status
// lists them all.
func (s *Server) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	var status procurement.Status
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		parsed, err := procurement.ParseStatus(raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "unknown_status", err.Error())
			return
		}
		status = parsed
	}
	orders, err := s.procurement.Orders(r.Context(), status)
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// statusForProcurement maps a procurement error to an HTTP status.
func statusForProcurement(err error) int {
	switch {
	case errors.Is(err, procurement.ErrOrderNotFound), errors.Is(err, procurement.ErrReceiptNotFound):
		return http.StatusNotFound
	case errors.Is(err, procurement.ErrOrderExists),
		errors.Is(err, procurement.ErrReceiptExists),
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
		errors.Is(err, procurement.ErrLineNetMismatch),
		errors.Is(err, procurement.ErrReceiptReferenceRequired),
		errors.Is(err, procurement.ErrReceiptDateRequired),
		errors.Is(err, procurement.ErrNoReceiptLines),
		errors.Is(err, procurement.ErrReceiptQuantity),
		errors.Is(err, procurement.ErrReceiptLineUnknown),
		errors.Is(err, procurement.ErrReceiptLineDuplicate),
		errors.Is(err, procurement.ErrOverReceipt):
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

// goodsReceiptLineRequest is one line of a delivery: how much of an order line arrived.
type goodsReceiptLineRequest struct {
	LineNo        int   `json:"line_no"`
	QuantityMilli int64 `json:"quantity_milli"`
}

// recordGoodsReceiptRequest is the payload of POST /api/purchase-orders/{id}/receipts.
type recordGoodsReceiptRequest struct {
	Reference  string                    `json:"reference"`
	ReceivedOn string                    `json:"received_on"`
	Lines      []goodsReceiptLineRequest `json:"lines"`
}

// recordGoodsReceipt records a delivery against an approved order (docs/12). The date is read at
// the edge like every other period, so a malformed one is a problem with the request rather than
// a domain refusal.
func (s *Server) recordGoodsReceipt(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req recordGoodsReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	receivedOn, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ReceivedOn))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_receipt_date", "received_on must be YYYY-MM-DD")
		return
	}
	lines := make([]procurement.ReceiptLine, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, procurement.ReceiptLine{LineNo: l.LineNo, QuantityMilli: l.QuantityMilli})
	}
	result, err := s.procurement.RecordReceipt(r.Context(), id, req.Reference, receivedOn, lines, actor(r))
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// listGoodsReceipts reads an order's goods-receipt state: every line's progress and the receipts
// recorded so far. An order with no delivery is a state with nothing received, not a 404.
func (s *Server) listGoodsReceipts(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := s.procurement.ReceiptState(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForProcurement(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
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
