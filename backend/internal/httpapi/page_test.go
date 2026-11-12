package httpapi

import (
	"net/http/httptest"
	"testing"
)

// TestPageParams covers the page a list route answers with, in the four shapes a request can have:
// nothing given, a page asked for, a limit past the bound, and a value that is not a number.
func TestPageParams(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
		wantCode   string
	}{
		{name: "nothing given opens the first page", query: "", wantLimit: defaultPageLimit, wantOffset: 0},
		{name: "a page is read as given", query: "?limit=25&offset=50", wantLimit: 25, wantOffset: 50},
		{name: "an empty limit is the default", query: "?limit=&offset=10", wantLimit: defaultPageLimit, wantOffset: 10},
		{name: "surrounding spaces are ignored", query: "?limit=%2025%20&offset=%205%20", wantLimit: 25, wantOffset: 5},
		{name: "a limit past the bound is clamped, not refused", query: "?limit=100000", wantLimit: maxPageLimit, wantOffset: 0},
		{name: "the bound itself is allowed", query: "?limit=200", wantLimit: maxPageLimit, wantOffset: 0},
		{name: "a non-numeric limit is refused", query: "?limit=fünf", wantCode: "invalid_limit"},
		{name: "a zero limit is refused", query: "?limit=0", wantCode: "invalid_limit"},
		{name: "a negative limit is refused", query: "?limit=-1", wantCode: "invalid_limit"},
		{name: "a non-numeric offset is refused", query: "?offset=zwölf", wantCode: "invalid_offset"},
		{name: "a negative offset is refused", query: "?offset=-5", wantCode: "invalid_offset"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/whatever"+c.query, nil)
			limit, offset, problem := pageParams(req)
			if c.wantCode != "" {
				if problem == nil {
					t.Fatalf("query %q was accepted, want %s", c.query, c.wantCode)
				}
				if problem.Code != c.wantCode {
					t.Fatalf("code = %q, want %q", problem.Code, c.wantCode)
				}
				if problem.Detail == "" {
					t.Fatal("a refusal without a detail tells a client nothing")
				}
				return
			}
			if problem != nil {
				t.Fatalf("query %q was refused: %s", c.query, problem.Detail)
			}
			if limit != c.wantLimit || offset != c.wantOffset {
				t.Fatalf("limit/offset = %d/%d, want %d/%d", limit, offset, c.wantLimit, c.wantOffset)
			}
		})
	}
}

// TestDefaultPageIsWithinTheBound keeps the two constants from being set to something contradictory,
// which would make every list route clamp its own default on the way out.
func TestDefaultPageIsWithinTheBound(t *testing.T) {
	if defaultPageLimit <= 0 || defaultPageLimit > maxPageLimit {
		t.Fatalf("default %d is not within 1..%d", defaultPageLimit, maxPageLimit)
	}
}
