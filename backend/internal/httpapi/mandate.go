package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// mandateService is the SEPA-mandate surface the HTTP layer needs.
type mandateService interface {
	CreateMandate(ctx context.Context, m kasse.Mandate, by string) (kasse.Mandate, error)
	Mandate(ctx context.Context, reference string) (kasse.Mandate, error)
}

// statusForMandate maps a mandate error to an HTTP status.
func statusForMandate(err error) int {
	switch {
	case errors.Is(err, kasse.ErrMandateNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrMandateReferenceExists):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrMandateReferenceRequired),
		errors.Is(err, kasse.ErrMandateReferenceTooLong),
		errors.Is(err, kasse.ErrNameRequired),
		errors.Is(err, kasse.ErrIBANInvalid),
		errors.Is(err, kasse.ErrDebtorBICInvalid),
		errors.Is(err, kasse.ErrSignatureDateRequired),
		errors.Is(err, kasse.ErrSequenceTypeUnknown),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type mandateRequest struct {
	Reference     string `json:"reference"`
	DebtorName    string `json:"debtor_name"`
	DebtorIBAN    string `json:"debtor_iban"`
	DebtorBIC     string `json:"debtor_bic"`
	SignatureDate string `json:"signature_date"` // YYYY-MM-DD
	SequenceType  string `json:"sequence_type"`
}

func (s *Server) createMandate(w http.ResponseWriter, r *http.Request) {
	var req mandateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	signatureDate, err := time.Parse(time.DateOnly, req.SignatureDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "signature_date must be YYYY-MM-DD")
		return
	}
	m, err := kasse.NewMandate(req.Reference, req.DebtorName, req.DebtorIBAN, req.DebtorBIC,
		signatureDate, kasse.MandateSequence(req.SequenceType))
	if err != nil {
		writeProblem(w, statusForMandate(err), err.Error())
		return
	}
	saved, err := s.mandates.CreateMandate(r.Context(), m, actor(r))
	if err != nil {
		writeProblem(w, statusForMandate(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getMandate(w http.ResponseWriter, r *http.Request) {
	m, err := s.mandates.Mandate(r.Context(), r.PathValue("reference"))
	if err != nil {
		writeProblem(w, statusForMandate(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}
