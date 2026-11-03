package httpapi

import (
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/report"
)

// reportCatalogue lists the reports this system renders. The worker role renders them; the API
// only needs to say which exist, and reads the same catalogue so a client cannot be told about
// a report that does not render (docs/25).
type reportCatalogue interface {
	Definitions() []report.Definition
}

// listReports returns the report definitions a client may ask for, so the ids it sends to
// POST /api/reports/renders come from the system rather than from a hardcoded list.
func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	if s.reports == nil {
		writeProblemCode(w, http.StatusServiceUnavailable, "report_catalogue_unconfigured",
			"no report catalogue is configured")
		return
	}
	writeJSON(w, http.StatusOK, s.reports.Definitions())
}
