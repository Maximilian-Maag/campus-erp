package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// einvoiceService is the e-invoicing surface the HTTP layer needs.
type einvoiceService interface {
	Create(ctx context.Context, inv core.Invoice, by string) (core.Invoice, error)
	ImportReceived(ctx context.Context, doc, by string) (core.Invoice, error)
	Get(ctx context.Context, id int64) (core.Invoice, error)
	Invoices(ctx context.Context) ([]einvoice.InvoiceSummary, error)
	Render(ctx context.Context, id int64) (string, error)
	RenderCII(ctx context.Context, id int64) (string, error)
	Match(ctx context.Context, id int64, by string) (core.Invoice, error)
}

// statusForEInvoice maps an e-invoice error to an HTTP status.
func statusForEInvoice(err error) int {
	switch {
	case errors.Is(err, einvoice.ErrNotFound), errors.Is(err, procurement.ErrOrderNotFound):
		return http.StatusNotFound
	case errors.Is(err, einvoice.ErrNumberExists), errors.Is(err, einvoice.ErrAlreadyMatched):
		return http.StatusConflict
	case errors.Is(err, procurement.ErrBadTransition):
		return http.StatusConflict
	case errors.Is(err, einvoice.ErrActorRequired),
		errors.Is(err, core.ErrNumberRequired),
		errors.Is(err, core.ErrIssueDateRequired),
		errors.Is(err, core.ErrCurrency),
		errors.Is(err, core.ErrSellerRequired),
		errors.Is(err, core.ErrBuyerRequired),
		errors.Is(err, core.ErrPartyNameRequired),
		errors.Is(err, core.ErrPartyCityRequired),
		errors.Is(err, core.ErrPartyPostalCode),
		errors.Is(err, core.ErrPartyCountry),
		errors.Is(err, core.ErrSellerVATRequired),
		errors.Is(err, core.ErrSellerEmailRequired),
		errors.Is(err, core.ErrBuyerReference),
		errors.Is(err, core.ErrPaymentMeansRequired),
		errors.Is(err, core.ErrPayeeIBANRequired),
		errors.Is(err, core.ErrNoLines),
		errors.Is(err, core.ErrLineDescription),
		errors.Is(err, core.ErrLineQuantity),
		errors.Is(err, core.ErrLineAmount),
		errors.Is(err, core.ErrVATCategoryRequired),
		errors.Is(err, core.ErrLineNetMismatch),
		errors.Is(err, core.ErrMalformed),
		errors.Is(err, einvoice.ErrNotInbound),
		errors.Is(err, procurement.ErrMatchAmountMismatch),
		errors.Is(err, core.ErrBadValue):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type einvoicePartyRequest struct {
	Name           string `json:"name"`
	Street         string `json:"street"`
	City           string `json:"city"`
	PostalCode     string `json:"postal_code"`
	Country        string `json:"country"`
	VatID          string `json:"vat_id"`
	EndpointID     string `json:"endpoint_id"`
	EndpointScheme string `json:"endpoint_scheme"`
	Email          string `json:"email"`
}

type einvoiceLineRequest struct {
	LineNo         int    `json:"line_no"`
	Description    string `json:"description"`
	QuantityMilli  int64  `json:"quantity_milli"`
	UnitCode       string `json:"unit_code"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	NetCents       int64  `json:"net_cents"`
	VatCategory    string `json:"vat_category"`
	VatRateBP      int64  `json:"vat_rate_bp"`
}

type createEInvoiceRequest struct {
	Number           string                `json:"number"`
	IssueDate        string                `json:"issue_date"`
	TypeCode         string                `json:"type_code"`
	Currency         string                `json:"currency"`
	BuyerReference   string                `json:"buyer_reference"`
	Seller           einvoicePartyRequest  `json:"seller"`
	Buyer            einvoicePartyRequest  `json:"buyer"`
	PaymentMeansCode string                `json:"payment_means_code"`
	PayeeIBAN        string                `json:"payee_iban"`
	PaymentTerms     string                `json:"payment_terms"`
	Lines            []einvoiceLineRequest `json:"lines"`
}

func (s *Server) createEInvoice(w http.ResponseWriter, r *http.Request) {
	var req createEInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	inv := core.Invoice{
		Number:           req.Number,
		TypeCode:         req.TypeCode,
		Currency:         req.Currency,
		BuyerReference:   req.BuyerReference,
		Seller:           toParty(req.Seller),
		Buyer:            toParty(req.Buyer),
		PaymentMeansCode: req.PaymentMeansCode,
		PayeeIBAN:        req.PayeeIBAN,
		PaymentTerms:     req.PaymentTerms,
	}
	if req.IssueDate != "" {
		d, err := time.Parse("2006-01-02", req.IssueDate)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "bad issue_date")
			return
		}
		inv.IssueDate = d
	}
	for _, l := range req.Lines {
		inv.Lines = append(inv.Lines, core.Line{
			LineNo: l.LineNo, Description: l.Description, QuantityMilli: l.QuantityMilli,
			UnitCode: l.UnitCode, UnitPriceCents: l.UnitPriceCents, NetCents: l.NetCents,
			VatCategory: l.VatCategory, VatRateBP: l.VatRateBP,
		})
	}
	saved, err := s.einvoices.Create(r.Context(), inv, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// receiveEInvoice parses, validates and stores an inbound XRechnung document.
func (s *Server) receiveEInvoice(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	saved, err := s.einvoices.ImportReceived(r.Context(), string(body), actor(r))
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// listEInvoices returns the invoices as summaries, so a screen can show which are still
// unmatched and match one without fetching each by id.
func (s *Server) listEInvoices(w http.ResponseWriter, r *http.Request) {
	invoices, err := s.einvoices.Invoices(r.Context())
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

// matchEInvoice runs the two-way match for an inbound invoice: it finds the purchase order the
// invoice's buyer reference names, checks the amounts agree, and converts that order's
// commitment into an actual (docs/09, docs/12).
func (s *Server) matchEInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	inv, err := s.einvoices.Match(r.Context(), id, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) getEInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	inv, err := s.einvoices.Get(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) getEInvoiceXML(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := s.einvoices.Render(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}

// getEInvoiceCII serves the same invoice as ZUGFeRD/Factur-X (CII) XML, next to the UBL one above:
// a counterpart that asks for a hybrid file gets the data part of it here (docs/09).
func (s *Server) getEInvoiceCII(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	doc, err := s.einvoices.RenderCII(r.Context(), id)
	if err != nil {
		writeDomainProblem(w, statusForEInvoice(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}

func toParty(p einvoicePartyRequest) core.Party {
	return core.Party{
		Name: p.Name, Street: p.Street, City: p.City, PostalCode: p.PostalCode,
		Country: p.Country, VatID: p.VatID, EndpointID: p.EndpointID,
		EndpointScheme: p.EndpointScheme, Email: p.Email,
	}
}
