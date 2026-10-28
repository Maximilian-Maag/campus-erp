package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
)

// lookupService is the lookup-catalogue surface the HTTP layer needs.
type lookupService interface {
	Upsert(ctx context.Context, v lookup.Value, by string) (lookup.Value, error)
	UpsertMany(ctx context.Context, values []lookup.Value, by string) ([]lookup.Value, error)
	Get(ctx context.Context, namespace, code string) (lookup.Value, error)
	List(ctx context.Context, namespace string) ([]lookup.Value, error)
	Seed(ctx context.Context, by string) (int, error)
}

// statusForLookup maps a lookup error to an HTTP status.
func statusForLookup(err error) int {
	switch {
	case errors.Is(err, lookup.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, lookup.ErrActorRequired),
		errors.Is(err, lookup.ErrNamespaceRequired),
		errors.Is(err, lookup.ErrCodeRequired),
		errors.Is(err, lookup.ErrShortTextRequired):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type lookupValueRequest struct {
	Code      string `json:"code"`
	ShortText string `json:"short_text"`
	LongText  string `json:"long_text"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}

type upsertLookupsRequest struct {
	Values []lookupValueRequest `json:"values"`
}

func (s *Server) listLookups(w http.ResponseWriter, r *http.Request) {
	values, err := s.lookups.List(r.Context(), r.PathValue("namespace"))
	if err != nil {
		writeProblem(w, statusForLookup(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *Server) getLookup(w http.ResponseWriter, r *http.Request) {
	v, err := s.lookups.Get(r.Context(), r.PathValue("namespace"), r.PathValue("code"))
	if err != nil {
		writeProblem(w, statusForLookup(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) upsertLookups(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	var req upsertLookupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	values := make([]lookup.Value, 0, len(req.Values))
	for _, v := range req.Values {
		values = append(values, lookup.Value{
			Namespace: ns, Code: v.Code, ShortText: v.ShortText,
			LongText: v.LongText, SortOrder: v.SortOrder, Active: v.Active,
		})
	}
	saved, err := s.lookups.UpsertMany(r.Context(), values, actor(r))
	if err != nil {
		writeProblem(w, statusForLookup(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) seedLookups(w http.ResponseWriter, r *http.Request) {
	n, err := s.lookups.Seed(r.Context(), actor(r))
	if err != nil {
		writeProblem(w, statusForLookup(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"seeded": n})
}
