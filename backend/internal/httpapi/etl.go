package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// etlRunner runs named ETL datasets (docs/19).
type etlRunner interface {
	Names() []string
	Run(ctx context.Context, name, by string) (etl.Report, error)
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
