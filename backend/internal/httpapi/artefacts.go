package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
)

// artefactKinds are the artefact families served over HTTP. Each is a directory the roles
// write into (docs/26). An allowlist, not a passthrough: without it the route would be a
// read-anything primitive over the whole store, and a new writer would silently widen it.
var artefactKinds = map[string]string{
	"dunning": "application/pdf",
	"reports": "application/pdf",
	"ledger":  "text/csv",
	"sepa":    "application/xml",
}

// ErrArtefactKind / ErrArtefactName are client errors: the request named something the
// route does not serve.
var (
	errArtefactKind = errors.New("unknown artefact kind")
	errArtefactName = errors.New("malformed artefact name")
)

// artefactKey turns the two path segments into a store key, refusing anything that is not
// a bare name inside a known kind - no separators, no traversal.
func artefactKey(kind, name string) (string, error) {
	if _, ok := artefactKinds[kind]; !ok {
		return "", fmt.Errorf("%w: %q", errArtefactKind, kind)
	}
	name = strings.TrimSpace(name)
	if name == "" || name != path.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("%w: %q", errArtefactName, name)
	}
	return kind + "/" + name, nil
}

// artefactContentType is the media type for a key. The kind already fixes it; the
// extension is the fallback for anything a kind does not pin (e.g. a CSV report).
func artefactContentType(key string) string {
	if contentType, ok := artefactKinds[strings.SplitN(key, "/", 2)[0]]; ok {
		if !strings.HasSuffix(key, ".csv") {
			return contentType
		}
	}
	switch strings.ToLower(path.Ext(key)) {
	case ".pdf":
		return "application/pdf"
	case ".csv":
		return "text/csv"
	case ".xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

// getArtefact serves a stored artefact. Keys are <kind>/<name>, so the route takes two
// segments and never a wildcard: the reachable set stays explicit (docs/26, docs/24).
func (s *Server) getArtefact(w http.ResponseWriter, r *http.Request) {
	key, err := artefactKey(r.PathValue("kind"), r.PathValue("name"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	content, err := s.artefacts.Get(r.Context(), key)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "no such artefact")
		return
	}
	w.Header().Set("Content-Type", artefactContentType(key))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
