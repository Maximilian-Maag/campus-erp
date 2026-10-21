package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// dunningService is the dunning surface the HTTP layer needs.
type dunningService interface {
	CreateLevel(ctx context.Context, l kasse.DunningLevel, by string) (kasse.DunningLevel, error)
	Level(ctx context.Context, year int, code string) (kasse.DunningLevel, error)
	Issue(ctx context.Context, year int, levelCode, claimReference string, outstandingCents int64, by string) (kasse.DunningNotice, error)
}

// statusForDunning maps a dunning error to an HTTP status.
func statusForDunning(err error) int {
	switch {
	case errors.Is(err, kasse.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, kasse.ErrLevelInactive), errors.Is(err, kasse.ErrLevelExists):
		return http.StatusConflict
	case errors.Is(err, kasse.ErrLevelCodeRequired),
		errors.Is(err, kasse.ErrBandsRequired),
		errors.Is(err, kasse.ErrBandsNotAscending),
		errors.Is(err, kasse.ErrOpenBandNotLast),
		errors.Is(err, kasse.ErrNegativeBand),
		errors.Is(err, kasse.ErrClaimRequired),
		errors.Is(err, kasse.ErrActorRequired),
		errors.Is(err, kasse.ErrAmountNotPositive),
		errors.Is(err, kasse.ErrNoDunningBand):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type createLevelRequest struct {
	Year        int    `json:"year"`
	Code        string `json:"code"`
	Description string `json:"description"`
	DaysAllowed int    `json:"days_allowed"`
	Active      *bool  `json:"active"`
	NextCode    string `json:"next_code"`
	Bands       []struct {
		UpToCents       int64 `json:"up_to_cents"`
		FeeCents        int64 `json:"fee_cents"`
		RateBasisPoints int64 `json:"rate_basis_points"`
	} `json:"bands"`
}

func (s *Server) createDunningLevel(w http.ResponseWriter, r *http.Request) {
	var req createLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	level := kasse.DunningLevel{
		Year:        req.Year,
		Code:        req.Code,
		Description: req.Description,
		DaysAllowed: req.DaysAllowed,
		Active:      req.Active == nil || *req.Active,
		NextCode:    req.NextCode,
	}
	for _, b := range req.Bands {
		level.Bands = append(level.Bands, kasse.DunningBand{
			UpToCents:       b.UpToCents,
			FeeCents:        b.FeeCents,
			RateBasisPoints: b.RateBasisPoints,
		})
	}
	saved, err := s.dunning.CreateLevel(r.Context(), level, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForDunning(err))
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) getDunningLevel(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		http.Error(w, "year must be an integer", http.StatusBadRequest)
		return
	}
	level, err := s.dunning.Level(r.Context(), year, r.PathValue("code"))
	if err != nil {
		http.Error(w, err.Error(), statusForDunning(err))
		return
	}
	writeJSON(w, http.StatusOK, level)
}

type issueNoticeRequest struct {
	Year             int    `json:"year"`
	LevelCode        string `json:"level_code"`
	ClaimReference   string `json:"claim_reference"`
	OutstandingCents int64  `json:"outstanding_cents"`
}

func (s *Server) issueDunningNotice(w http.ResponseWriter, r *http.Request) {
	var req issueNoticeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	notice, err := s.dunning.Issue(r.Context(), req.Year, req.LevelCode, req.ClaimReference, req.OutstandingCents, actor(r))
	if err != nil {
		http.Error(w, err.Error(), statusForDunning(err))
		return
	}
	writeJSON(w, http.StatusCreated, notice)
}
