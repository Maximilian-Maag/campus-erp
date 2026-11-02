package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

// exportQueue is the enqueue seam: it hands an export to the worker role instead of
// running it inline, and returns the queued job's id. It is an interface so the
// transport is testable with a stub and so River stays out of this package.
type exportQueue interface {
	EnqueueStatisticsExport(ctx context.Context, state, from, to, actor string) (int64, error)
}

// statisticsExportRequest is the payload of POST /api/statistics/exports.
type statisticsExportRequest struct {
	State string `json:"state"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// statisticsExportResponse is the accepted job.
type statisticsExportResponse struct {
	JobID int64 `json:"job_id"`
}

// requestStatisticsExport queues a per-state statistics export (docs/27). The work runs
// on the worker role, so this handler only accepts the request and returns the job id;
// the file is stored under `statistics/<state>-<from>-<to>.csv` and is then reachable
// through GET /api/artefacts/statistics/<name>.
func (s *Server) requestStatisticsExport(w http.ResponseWriter, r *http.Request) {
	if s.exports == nil {
		writeProblem(w, http.StatusServiceUnavailable, "export queue is not configured")
		return
	}
	var req statisticsExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	state := strings.TrimSpace(req.State)
	if state == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "state is required")
		return
	}
	from, err := parseExportDate(req.From)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "from must be YYYY-MM-DD")
		return
	}
	to, err := parseExportDate(req.To)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "to must be YYYY-MM-DD")
		return
	}
	id, err := s.exports.EnqueueStatisticsExport(r.Context(), state, from, to, actor(r))
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "could not queue the export")
		return
	}
	writeJSON(w, http.StatusAccepted, statisticsExportResponse{JobID: id})
}

// parseExportDate normalises an export's period boundary to the layout the job carries,
// so a caller's date is accepted here rather than failing later on the worker.
func parseExportDate(v string) (string, error) {
	t, err := time.Parse(jobs.PeriodLayout, strings.TrimSpace(v))
	if err != nil {
		return "", err
	}
	return t.Format(jobs.PeriodLayout), nil
}
