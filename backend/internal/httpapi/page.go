package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// Paging for the list routes (docs/15).
//
// A list that grows with use — the invoices of a running installation, the movements of a book —
// must not answer with the whole table: the request gets slower, the screen builds a DOM nobody
// reads, and the answer is a page nobody asked for. So a list route takes `limit` and `offset`, and
// the screen asks for the next page when the operator wants it.
//
// The defaults are the page a screen opens with. The bound is what keeps one client from asking for
// everything at once; a limit past it is clamped rather than refused, because the request is
// understandable and the answer is still a page, just the largest one. Anything that is not a
// non-negative integer *is* refused, with a machine code, rather than quietly falling back to the
// default: a client that sends `offset=zwölf` has a bug, and hiding it would make the symptom (a
// list that never advances) much harder to find than the cause.
const (
	// defaultPageLimit is the page a list route answers with when no limit is given.
	defaultPageLimit = 50
	// maxPageLimit is the largest page a list route will answer with.
	maxPageLimit = 200
)

// pageProblem is a refused page parameter, carrying the machine code a client branches on.
type pageProblem struct {
	Code   string
	Detail string
}

// pageParams reads `limit` and `offset` from a list request, applying the defaults and the bound.
func pageParams(r *http.Request) (limit, offset int, problem *pageProblem) {
	limit = defaultPageLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, 0, &pageProblem{Code: "invalid_limit", Detail: "limit must be a positive integer"}
		}
		limit = n
		if limit > maxPageLimit {
			limit = maxPageLimit
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, 0, &pageProblem{Code: "invalid_offset", Detail: "offset must be zero or a positive integer"}
		}
		offset = n
	}
	return limit, offset, nil
}
