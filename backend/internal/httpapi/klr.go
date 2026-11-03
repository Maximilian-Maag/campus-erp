package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// klrService is the KLR surface the HTTP layer needs.
type klrService interface {
	CreateCostCenter(ctx context.Context, number, name, by string) (klr.CostCenter, error)
	CostCenters(ctx context.Context) ([]klr.CostCenter, error)
	CreateCostUnit(ctx context.Context, number, name, by string) (klr.CostUnit, error)
	Post(ctx context.Context, costCenterID int64, costUnitID *int64, kind klr.Kind, amountCents int64, businessDate time.Time, reference, by string) (klr.Record, error)
	Report(ctx context.Context, costCenterID int64) (klr.Report, error)
	DefineAllocation(ctx context.Context, sourceCenterID, targetCenterID, shareBP int64, validFrom time.Time, validTo *time.Time, by string) (klr.Allocation, error)
	Allocations(ctx context.Context, sourceCenterID int64) ([]klr.Allocation, error)
	Allocate(ctx context.Context, sourceCenterID int64, from, to time.Time, by string) (klr.Run, error)
}

// statusForKLR maps a KLR error to an HTTP status.
func statusForKLR(err error) int {
	switch {
	case errors.Is(err, klr.ErrCostCenterNotFound), errors.Is(err, klr.ErrCostUnitNotFound):
		return http.StatusNotFound
	case errors.Is(err, klr.ErrCostCenterExists), errors.Is(err, klr.ErrCostUnitExists):
		return http.StatusConflict
	case errors.Is(err, klr.ErrAlreadyAllocated):
		return http.StatusConflict
	case errors.Is(err, klr.ErrActorRequired),
		errors.Is(err, klr.ErrNumberRequired),
		errors.Is(err, klr.ErrNameRequired),
		errors.Is(err, klr.ErrAmountNotPositive),
		errors.Is(err, klr.ErrUnknownKind),
		errors.Is(err, klr.ErrBusinessDateRequired),
		errors.Is(err, klr.ErrAllocationSelf),
		errors.Is(err, klr.ErrAllocationShare),
		errors.Is(err, klr.ErrAllocationPeriod),
		errors.Is(err, klr.ErrAllocationRunPeriod),
		errors.Is(err, klr.ErrAllocationRunNoTargets),
		errors.Is(err, klr.ErrSharesIncomplete),
		errors.Is(err, klr.ErrNothingToAllocate):
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

// listCostCenters reads every cost centre, which is what a screen needs before it can ask about
// one.
func (s *Server) listCostCenters(w http.ResponseWriter, r *http.Request) {
	centers, err := s.klr.CostCenters(r.Context())
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, centers)
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
	CostUnitID *int64 `json:"cost_unit_id"`
	Kind       string `json:"kind"`
	// BusinessDate is the period the record belongs to ("2026-04-30"). Empty means today: the
	// edge decides that, and the domain is given an explicit date.
	BusinessDate string `json:"business_date"`
	AmountCents  int64  `json:"amount_cents"`
	Reference    string `json:"reference"`
}

// businessDate reads a record's period, defaulting to today when the caller sent none.
func businessDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	parsed, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
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
	date, err := businessDate(req.BusinessDate)
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "business_date must be a date (YYYY-MM-DD)")
		return
	}
	rec, err := s.klr.Post(r.Context(), id, req.CostUnitID, klr.Kind(req.Kind), req.AmountCents, date, req.Reference, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

type defineAllocationRequest struct {
	TargetCenterID int64  `json:"target_center_id"`
	ShareBP        int64  `json:"share_bp"`
	ValidFrom      string `json:"valid_from"`
	ValidTo        string `json:"valid_to"`
}

// defineAllocation stores a distribution key (docs/13). The shares of a cost centre have to add
// up to 10000 basis points when a run uses them, not here: keys are valid for a range, so the
// set in force changes over time.
func (s *Server) defineAllocation(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req defineAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ValidFrom))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_from must be a date (YYYY-MM-DD)")
		return
	}
	var to *time.Time
	if raw := strings.TrimSpace(req.ValidTo); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_to must be a date (YYYY-MM-DD)")
			return
		}
		to = &parsed
	}
	key, err := s.klr.DefineAllocation(r.Context(), sourceID, req.TargetCenterID, req.ShareBP, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) listAllocations(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	keys, err := s.klr.Allocations(r.Context(), sourceID)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

type runAllocationRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// runAllocation distributes a cost centre's period costs over its keys.
func (s *Server) runAllocation(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req runAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.From))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "from must be a date (YYYY-MM-DD)")
		return
	}
	to, err := time.Parse(time.DateOnly, strings.TrimSpace(req.To))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "to must be a date (YYYY-MM-DD)")
		return
	}
	run, err := s.klr.Allocate(r.Context(), sourceID, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
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
