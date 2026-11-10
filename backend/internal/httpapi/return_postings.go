package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/jobs"
)

// returnPostingsService is the return-postings surface the HTTP layer needs: the read the
// screen lists, so it and the queued export see the same set (docs/30).
type returnPostingsService interface {
	Returns(ctx context.Context, from, to time.Time) ([]kasse.StatementLine, error)
}

// statusForReturns maps a return-postings error to an HTTP status.
func statusForReturns(err error) int {
	switch {
	case errors.Is(err, kasse.ErrReturnsPeriodInvalid), errors.Is(err, kasse.ErrReturnsAccountsIncomplete):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// listReturnedLines reads the returning bank-statement lines of a period — the lines a
// return-postings export is built from, so the operator sees what the file will carry.
// The period is a query parameter and required: "every return ever" is not a period an
// operator states, and an empty answer must mean "none in the period" rather than
// "no period given".
func (s *Server) listReturnedLines(w http.ResponseWriter, r *http.Request) {
	if s.returns == nil {
		writeProblemCode(w, http.StatusServiceUnavailable, "returns_unconfigured", "return postings are not configured")
		return
	}
	from, to, ok := queryPeriod(w, r)
	if !ok {
		return
	}
	lines, err := s.returns.Returns(r.Context(), from, to)
	if err != nil {
		writeProblem(w, statusForReturns(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

// requestReturnPostings queues the return-postings export for a period (docs/30). The file
// lands under `ledger/return-postings-<timestamp>.csv`, next to the posting export it
// reverses.
func (s *Server) requestReturnPostings(w http.ResponseWriter, r *http.Request) {
	var req ledgerExportRequest
	if !s.decodeExportRequest(w, r, &req) {
		return
	}
	from, to, ok := s.exportPeriod(w, req.From, req.To)
	if !ok {
		return
	}
	s.enqueueExport(w, func() (int64, error) {
		return s.exports.EnqueueReturnPostings(r.Context(), from, to, actor(r))
	})
}

// queryPeriod reads a required [from, to] period in YYYY-MM-DD from the query string,
// writing the problem response itself when either boundary is missing or unreadable.
func queryPeriod(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	from, err := time.Parse(jobs.PeriodLayout, r.URL.Query().Get("from"))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_period", "from must be YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	to, err := time.Parse(jobs.PeriodLayout, r.URL.Query().Get("to"))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_period", "to must be YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}
