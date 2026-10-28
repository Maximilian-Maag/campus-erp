package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// klrService is the KLR surface the HTTP layer needs.
type klrService interface {
	CreateCostCenter(ctx context.Context, number, name, by string) (klr.CostCenter, error)
	CreateCostUnit(ctx context.Context, number, name, by string) (klr.CostUnit, error)
	Post(ctx context.Context, costCenterID int64, costUnitID *int64, kind klr.Kind, amountCents int64, reference, by string) (klr.Record, error)
	Report(ctx context.Context, costCenterID int64) (klr.Report, error)
}

// statusForKLR maps a KLR error to an HTTP status.
func statusForKLR(err error) int {
	switch {
	case errors.Is(err, klr.ErrCostCenterNotFound), errors.Is(err, klr.ErrCostUnitNotFound):
		return http.StatusNotFound
	case errors.Is(err, klr.ErrCostCenterExists), errors.Is(err, klr.ErrCostUnitExists):
		return http.StatusConflict
	case errors.Is(err, klr.ErrActorRequired),
		errors.Is(err, klr.ErrNumberRequired),
		errors.Is(err, klr.ErrNameRequired),
		errors.Is(err, klr.ErrAmountNotPositive),
		errors.Is(err, klr.ErrUnknownKind):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type klrNamedRequest struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

func (s *Server) createCostCenter(w http.ResponseWriter, r *http.Request) {
	var req klrNamedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	center, err := s.klr.CreateCostCenter(r.Context(), req.Number, req.Name, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, center)
}

func (s *Server) createCostUnit(w http.ResponseWriter, r *http.Request) {
	var req klrNamedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	unit, err := s.klr.CreateCostUnit(r.Context(), req.Number, req.Name, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

type klrRecordRequest struct {
	CostUnitID  *int64 `json:"cost_unit_id"`
	Kind        string `json:"kind"`
	AmountCents int64  `json:"amount_cents"`
	Reference   string `json:"reference"`
}

func (s *Server) postAccountingRecord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req klrRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	rec, err := s.klr.Post(r.Context(), id, req.CostUnitID, klr.Kind(req.Kind), req.AmountCents, req.Reference, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) getKLRReport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := s.klr.Report(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
