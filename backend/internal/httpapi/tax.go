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

// taxService is the tax-office surface the HTTP layer needs.
type taxService interface {
	CreateVATType(ctx context.Context, v kasse.VATType, by string) (kasse.VATType, error)
	VATType(ctx context.Context, code string) (kasse.VATType, error)
	CreateTaxOffice(ctx context.Context, t kasse.TaxOffice, by string) (kasse.TaxOffice, error)
	CreateFoundation(ctx context.Context, f kasse.Foundation, by string) (kasse.Foundation, error)
	Foundations(ctx context.Context, year int) ([]kasse.Foundation, error)
}

// statusForTax maps a tax-data error to an HTTP status.
func statusForTax(err error) int {
	switch {
	case errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrVATCodeExists),
		errors.Is(err, kasse.ErrTaxOfficeExists),
		errors.Is(err, kasse.ErrFoundationExists):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrVATCodeRequired),
		errors.Is(err, kasse.ErrVATCodeTooLong),
		errors.Is(err, kasse.ErrVATRateOutOfRange),
		errors.Is(err, kasse.ErrVATKindUnknown),
		errors.Is(err, kasse.ErrIBANInvalid),
		errors.Is(err, kasse.ErrNumberRequired),
		errors.Is(err, kasse.ErrNameRequired),
		errors.Is(err, kasse.ErrBadYear),
		errors.Is(err, kasse.ErrActorRequired),
		errors.Is(err, kasse.ErrAmountNegative):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type vatTypeRequest struct {
	Code            string `json:"code"`
	Description     string `json:"description"`
	RateBasisPoints int64  `json:"rate_basis_points"`
	Kind            string `json:"kind"`
}

func (s *Server) createVATType(w http.ResponseWriter, r *http.Request) {
	var req vatTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	vat, err := kasse.NewVATType(req.Code, req.Description, req.RateBasisPoints, kasse.VATKind(req.Kind))
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	saved, err := s.tax.CreateVATType(r.Context(), vat, actor(r))
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getVATType(w http.ResponseWriter, r *http.Request) {
	vat, err := s.tax.VATType(r.Context(), r.PathValue("code"))
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vat)
}

type taxOfficeRequest struct {
	Year          int    `json:"year"`
	Number        string `json:"number"`
	Name          string `json:"name"`
	Street        string `json:"street"`
	PostalCode    string `json:"postal_code"`
	City          string `json:"city"`
	AccountHolder string `json:"account_holder"`
	IBAN          string `json:"iban"`
	BIC           string `json:"bic"`
}

func (s *Server) createTaxOffice(w http.ResponseWriter, r *http.Request) {
	var req taxOfficeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	office, err := kasse.NewTaxOffice(req.Year, req.Number, req.Name, req.Street,
		req.PostalCode, req.City, req.AccountHolder, req.IBAN, req.BIC)
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	saved, err := s.tax.CreateTaxOffice(r.Context(), office, actor(r))
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

type foundationRequest struct {
	Year        int    `json:"year"`
	Number      string `json:"number"`
	Description string `json:"description"`
	CarryOver   bool   `json:"carry_over"`
}

func (s *Server) createFoundation(w http.ResponseWriter, r *http.Request) {
	var req foundationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	foundation, err := kasse.NewFoundation(req.Year, req.Number, req.Description, req.CarryOver)
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	saved, err := s.tax.CreateFoundation(r.Context(), foundation, actor(r))
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// listFoundations reads the foundation references. The year filter is the server's: ?year=2026
// narrows to one year, and no year lists every year.
func (s *Server) listFoundations(w http.ResponseWriter, r *http.Request) {
	year := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("year")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_year", "year must be an integer")
			return
		}
		year = parsed
	}
	foundations, err := s.tax.Foundations(r.Context(), year)
	if err != nil {
		writeProblem(w, statusForTax(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, foundations)
}
