package kasse

import (
	"strings"
	"time"
)

// nowFunc is the clock used when a service stamps a record. It is a variable so
// tests can pin time deterministically.
var nowFunc = time.Now

// requireActor rejects a blank acting user.
func requireActor(by string) error {
	if strings.TrimSpace(by) == "" {
		return ErrActorRequired
	}
	return nil
}
