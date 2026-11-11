package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// mandateService is the SEPA-mandate surface the HTTP layer needs.
type mandateService interface {
	CreateMandate(ctx context.Context, m kasse.Mandate, by string) (kasse.Mandate, error)
	Mandate(ctx context.Context, reference string) (kasse.Mandate, error)
	Mandates(ctx context.Context, activeOnly bool, seq kasse.MandateSequence) ([]kasse.Mandate, error)
	RetireMandate(ctx context.Context, reference, by string) (kasse.Mandate, error)
}

// statusForMandate maps a mandate error to an HTTP status.
func statusForMandate(err error) int {
	switch {
	case errors.Is(err, kasse.ErrMandateNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrMandateReferenceExists), errors.Is(err, kasse.ErrMandateRetired):
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
		writeDomainProblem(w, statusForMandate(err), err)
		return
	}
	saved, err := s.mandates.CreateMandate(r.Context(), m, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForMandate(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getMandate(w http.ResponseWriter, r *http.Request) {
	m, err := s.mandates.Mandate(r.Context(), r.PathValue("reference"))
	if err != nil {
		writeDomainProblem(w, statusForMandate(err), err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// retireMandate withdraws a mandate's authorisation: the debtor revoked it. Nothing is deleted —
// the mandate keeps its history and its reference, and the state is `active = false` (docs/21).
func (s *Server) retireMandate(w http.ResponseWriter, r *http.Request) {
	retired, err := s.mandates.RetireMandate(r.Context(), r.PathValue("reference"), actor(r))
	if err != nil {
		writeDomainProblem(w, statusForMandate(err), err)
		return
	}
	writeJSON(w, http.StatusOK, retired)
}

// listMandates reads the mandates, by reference. The filters are the server's:
// ?active=true narrows to the live ones and ?sequence_type= to those collecting
// under one sequence type, so a screen that offers the mandates a run can gather
// never holds every mandate to show a few.
func (s *Server) listMandates(w http.ResponseWriter, r *http.Request) {
	activeOnly := false
	switch r.URL.Query().Get("active") {
	case "":
	case "true":
		activeOnly = true
	case "false":
	default:
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_active", "active must be true or false")
		return
	}
	seq := kasse.MandateSequence(strings.TrimSpace(r.URL.Query().Get("sequence_type")))
	if seq != "" && !kasse.ValidSequenceType(seq) {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_sequence_type", "sequence_type must be FRST, RCUR, OOFF or FNAL")
		return
	}
	mandates, err := s.mandates.Mandates(r.Context(), activeOnly, seq)
	if err != nil {
		writeDomainProblem(w, statusForMandate(err), err)
		return
	}
	writeJSON(w, http.StatusOK, mandates)
}
