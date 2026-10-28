package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestActorFromHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := actor(r); got != "anonymous" {
		t.Errorf("actor() = %q, want anonymous", got)
	}
	r.Header.Set("X-Actor", "alice")
	if got := actor(r); got != "alice" {
		t.Errorf("actor() = %q, want alice", got)
	}
}
