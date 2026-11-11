package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// collectionRunService is the collection-run surface the HTTP layer needs.
type collectionRunService interface {
	CreateCollectionRun(ctx context.Context, reference string, accountID int64, creditorID string,
		collectionDate time.Time, sequenceType kasse.MandateSequence, localInstrument string,
		items []kasse.CollectionItem, by string) (kasse.CollectionRun, error)
	CreateCollectionRunFromReceivables(ctx context.Context, reference string, accountID int64,
		creditorID string, collectionDate time.Time, sequenceType kasse.MandateSequence,
		localInstrument string, dueBy time.Time, by string) (kasse.CollectionRun, error)
	CollectionRun(ctx context.Context, reference string) (kasse.CollectionRun, error)
	CollectionRunFile(ctx context.Context, reference string) (string, error)
}

// statusForCollectionRun maps a collection-run error to an HTTP status.
func statusForCollectionRun(err error) int {
	switch {
	case errors.Is(err, kasse.ErrCollectionRunNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrRunReferenceExists):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrRunReferenceRequired),
		errors.Is(err, kasse.ErrCashAccountRequired),
		errors.Is(err, kasse.ErrCollectionCreditorIDRequired),
		errors.Is(err, kasse.ErrCollectionDateRequired),
		errors.Is(err, kasse.ErrSequenceTypeUnknown),
		errors.Is(err, kasse.ErrLocalInstrumentUnknown),
		errors.Is(err, kasse.ErrNoCollectionItems),
		errors.Is(err, kasse.ErrMandateReferenceRequired),
		errors.Is(err, kasse.ErrDuplicateMandate),
		errors.Is(err, kasse.ErrEndToEndIDRequired),
		errors.Is(err, kasse.ErrAmountNotPositive),
		errors.Is(err, kasse.ErrMandateNotActive),
		errors.Is(err, kasse.ErrMandateSequenceMismatch),
		errors.Is(err, kasse.ErrMandateNotFound),
		errors.Is(err, kasse.ErrReceivableDueByRequired),
		errors.Is(err, kasse.ErrReceivableDueByAfterCollection),
		errors.Is(err, kasse.ErrReceivableCurrencyMismatch),
		errors.Is(err, kasse.ErrReceivableNotOpen),
		errors.Is(err, kasse.ErrActorRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type collectionItemRequest struct {
	MandateReference string `json:"mandate_reference"`
	EndToEndID       string `json:"end_to_end_id"`
	AmountCents      int64  `json:"amount_cents"`
	Purpose          string `json:"purpose"`
}

type collectionRunRequest struct {
	Reference       string                  `json:"reference"`
	AccountID       int64                   `json:"account_id"`
	CreditorID      string                  `json:"creditor_id"`
	CollectionDate  string                  `json:"collection_date"` // YYYY-MM-DD
	SequenceType    string                  `json:"sequence_type"`
	LocalInstrument string                  `json:"local_instrument"`
	Items           []collectionItemRequest `json:"items"`
}

func (s *Server) createCollectionRun(w http.ResponseWriter, r *http.Request) {
	var req collectionRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	collectionDate, err := time.Parse(time.DateOnly, req.CollectionDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "collection_date must be YYYY-MM-DD")
		return
	}
	items := make([]kasse.CollectionItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, kasse.CollectionItem{
			MandateReference: it.MandateReference,
			EndToEndID:       it.EndToEndID,
			AmountCents:      it.AmountCents,
			Purpose:          it.Purpose,
		})
	}
	run, err := s.collectionRuns.CreateCollectionRun(r.Context(), req.Reference, req.AccountID, req.CreditorID,
		collectionDate, kasse.MandateSequence(req.SequenceType), req.LocalInstrument, items, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForCollectionRun(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

type collectionRunFromReceivablesRequest struct {
	Reference       string `json:"reference"`
	AccountID       int64  `json:"account_id"`
	CreditorID      string `json:"creditor_id"`
	CollectionDate  string `json:"collection_date"` // YYYY-MM-DD
	SequenceType    string `json:"sequence_type"`
	LocalInstrument string `json:"local_instrument"`
	DueBy           string `json:"due_by"` // YYYY-MM-DD
}

// createCollectionRunFromReceivables selects the open receivables due by due_by
// and collects them, rather than taking the items from the caller.
func (s *Server) createCollectionRunFromReceivables(w http.ResponseWriter, r *http.Request) {
	var req collectionRunFromReceivablesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	collectionDate, err := time.Parse(time.DateOnly, req.CollectionDate)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "collection_date must be YYYY-MM-DD")
		return
	}
	dueBy, err := time.Parse(time.DateOnly, req.DueBy)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "due_by must be YYYY-MM-DD")
		return
	}
	run, err := s.collectionRuns.CreateCollectionRunFromReceivables(r.Context(), req.Reference, req.AccountID,
		req.CreditorID, collectionDate, kasse.MandateSequence(req.SequenceType), req.LocalInstrument, dueBy, actor(r))
	if err != nil {
		writeDomainProblem(w, statusForCollectionRun(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getCollectionRunFile(w http.ResponseWriter, r *http.Request) {
	file, err := s.collectionRuns.CollectionRunFile(r.Context(), r.PathValue("reference"))
	if err != nil {
		writeDomainProblem(w, statusForCollectionRun(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(file))
}
