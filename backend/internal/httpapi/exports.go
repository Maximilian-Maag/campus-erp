package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

// exportQueue is the enqueue seam: it hands an export or a render to the worker role
// instead of running it inline, and returns the queued job's id. It is an interface so
// the transport is testable with a stub and so River stays out of this package.
type exportQueue interface {
	EnqueueStatisticsExport(ctx context.Context, state, from, to, actor string) (int64, error)
	EnqueueLedgerExport(ctx context.Context, from, to, actor string) (int64, error)
	EnqueueReturnPostings(ctx context.Context, from, to, actor string) (int64, error)
	EnqueueReportRender(ctx context.Context, reportID, from, to, actor string) (int64, error)
	EnqueueStatementImport(ctx context.Context, accountID int64, content, actor string) (int64, error)
	EnqueueSepaExport(ctx context.Context, runID int64, actor string) (int64, error)
}

// statisticsExportRequest is the payload of POST /api/statistics/exports.
type statisticsExportRequest struct {
	State string `json:"state"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// ledgerExportRequest is the payload of POST /api/ledger/exports.
type ledgerExportRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// statementImportRequest is the payload of POST /api/bank-statements/imports.
type statementImportRequest struct {
	AccountID int64  `json:"account_id"`
	Content   string `json:"content"`
}

// reportRenderRequest is the payload of POST /api/reports/renders.
type reportRenderRequest struct {
	ReportID string `json:"report_id"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// exportQueuedResponse is an accepted job: the work runs on the worker role, so the
// caller gets the job id and follows the artefact it produces.
type exportQueuedResponse struct {
	JobID int64 `json:"job_id"`
}

// requestStatisticsExport queues a per-state statistics export (docs/27). The file lands
// under `statistics/<state>-<from>-<to>.csv` and is then reachable through
// GET /api/artefacts/statistics/<name>.
func (s *Server) requestStatisticsExport(w http.ResponseWriter, r *http.Request) {
	var req statisticsExportRequest
	if !s.decodeExportRequest(w, r, &req) {
		return
	}
	state := strings.TrimSpace(req.State)
	if state == "" {
		writeProblemCode(w, http.StatusUnprocessableEntity, "state_required", "state is required")
		return
	}
	from, to, ok := s.exportPeriod(w, req.From, req.To)
	if !ok {
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueStatisticsExport(r.Context(), state, from, to, actor(r))
	})
}

// requestLedgerExport queues a ledger posting export for a period (docs/23). The file
// lands under `ledger/ledger-postings-<timestamp>.csv`.
func (s *Server) requestLedgerExport(w http.ResponseWriter, r *http.Request) {
	var req ledgerExportRequest
	if !s.decodeExportRequest(w, r, &req) {
		return
	}
	from, to, ok := s.exportPeriod(w, req.From, req.To)
	if !ok {
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueLedgerExport(r.Context(), from, to, actor(r))
	})
}

// requestReportRender queues a report render by id over a period (docs/25). The PDF lands
// under `reports/<report id>-<from>-<to>.pdf`.
func (s *Server) requestReportRender(w http.ResponseWriter, r *http.Request) {
	var req reportRenderRequest
	if !s.decodeExportRequest(w, r, &req) {
		return
	}
	reportID := strings.TrimSpace(req.ReportID)
	if reportID == "" {
		writeProblemCode(w, http.StatusUnprocessableEntity, "report_id_required", "report_id is required")
		return
	}
	from, to, ok := s.exportPeriod(w, req.From, req.To)
	if !ok {
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueReportRender(r.Context(), reportID, from, to, actor(r))
	})
}

// requestStatementImport queues a bank-statement import, so a large file does not hold the
// request open (POST /api/bank-statements is the synchronous counterpart). The bytes travel
// with the job, so a retry re-imports the same file.
func (s *Server) requestStatementImport(w http.ResponseWriter, r *http.Request) {
	var req statementImportRequest
	if !s.decodeExportRequest(w, r, &req) {
		return
	}
	if req.AccountID <= 0 {
		writeProblemCode(w, http.StatusUnprocessableEntity, "account_required", "account_id is required")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeProblemCode(w, http.StatusUnprocessableEntity, "content_required", "content is required")
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueStatementImport(r.Context(), req.AccountID, req.Content, actor(r))
	})
}

// requestSepaExport queues the delivery of a payment run's already-rendered SEPA file to the
// bank channel; the run id is the path parameter.
func (s *Server) requestSepaExport(w http.ResponseWriter, r *http.Request) {
	if s.exports == nil {
		writeProblemCode(w, http.StatusServiceUnavailable, "export_queue_unconfigured", "export queue is not configured")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeProblemCode(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueSepaExport(r.Context(), id, actor(r))
	})
}

// decodeExportRequest checks the queue is configured and decodes the body, writing the
// problem response itself. It reports whether the caller should continue.
func (s *Server) decodeExportRequest(w http.ResponseWriter, r *http.Request, into any) bool {
	if s.exports == nil {
		writeProblemCode(w, http.StatusServiceUnavailable, "export_queue_unconfigured", "export queue is not configured")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return false
	}
	return true
}

// exportPeriod validates both period boundaries at the edge, so a doomed job is not
// queued; the worker re-validates, because a job argument can be persisted and replayed
// by hand.
func (s *Server) exportPeriod(w http.ResponseWriter, from, to string) (string, string, bool) {
	f, err := parseExportDate(from)
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_period", "from must be YYYY-MM-DD")
		return "", "", false
	}
	t, err := parseExportDate(to)
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_period", "to must be YYYY-MM-DD")
		return "", "", false
	}
	return f, t, true
}

// enqueueExport runs a queue call and writes the accepted (or failed) response.
func (s *Server) enqueueExport(w http.ResponseWriter, enqueue func() (int64, error)) {
	id, err := enqueue()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "could not queue the export")
		return
	}
	writeJSON(w, http.StatusAccepted, exportQueuedResponse{JobID: id})
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
