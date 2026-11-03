package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// etlRunner lists, runs and reads back named ETL datasets (docs/19).
type etlRunner interface {
	Names() []string
	Run(ctx context.Context, name, by string) (etl.Report, error)
	History(ctx context.Context, name string, limit int) ([]etl.Run, error)
}

// statusForETL maps an ETL error to an HTTP status.
func statusForETL(err error) int {
	switch {
	case errors.Is(err, etl.ErrUnknownDataset):
		return http.StatusNotFound
	case errors.Is(err, etl.ErrActorRequired), errors.Is(err, etl.ErrReconcile):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// datasetsResponse is the payload of GET /api/etl/datasets.
type datasetsResponse struct {
	Datasets []string `json:"datasets"`
}

func (s *Server) listETLDatasets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, datasetsResponse{Datasets: s.etl.Names()})
}

type etlRunRequest struct {
	Dataset string `json:"dataset"`
}

// etlRunsResponse is the payload of GET /api/etl/runs.
type etlRunsResponse struct {
	Runs []etl.Run `json:"runs"`
}

// listETLRuns returns a dataset's recent runs — or every dataset's when none is named —
// read from the audit stream the runs are already recorded in (docs/19).
func (s *Server) listETLRuns(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_limit", "limit must be a positive integer")
			return
		}
		limit = n
	}
	runs, err := s.etl.History(r.Context(), r.URL.Query().Get("dataset"), limit)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "could not read the run history")
		return
	}
	writeJSON(w, http.StatusOK, etlRunsResponse{Runs: runs})
}

func (s *Server) runETLDataset(w http.ResponseWriter, r *http.Request) {
	var req etlRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	report, err := s.etl.Run(r.Context(), req.Dataset, actor(r))
	if err != nil {
		writeProblem(w, statusForETL(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
