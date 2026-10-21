// Package audit implements an append-only, hash-chained audit stream (ADR-0006).
//
// Every state change in every module must be recorded here. The chain makes
// tampering detectable: altering any event invalidates all later hashes.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// GenesisHash is the predecessor hash of the first event.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ErrChainBroken is returned by Log.Verify when the chain does not recompute.
var ErrChainBroken = errors.New("audit: chain broken")

// Event is one immutable audit record.
type Event struct {
	Seq      uint64    `json:"seq"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	Subject  string    `json:"subject"`
	Before   string    `json:"before,omitempty"`
	After    string    `json:"after,omitempty"`
	PrevHash string    `json:"prev_hash"`
	Hash     string    `json:"hash"`
}

// Log is an in-memory append-only audit stream. Persistence is a later concern.
type Log struct {
	events []Event
}

// Append records an event, assigning Seq, PrevHash and Hash.
func (l *Log) Append(e Event) Event {
	if n := len(l.events); n > 0 {
		e.PrevHash = l.events[n-1].Hash
	} else {
		e.PrevHash = GenesisHash
	}
	e.Seq = uint64(len(l.events)) + 1
	e.Hash = hashEvent(e)
	l.events = append(l.events, e)
	return e
}

// Len reports the number of recorded events.
func (l *Log) Len() int { return len(l.events) }

// Events returns a copy of the recorded events.
func (l *Log) Events() []Event { return append([]Event(nil), l.events...) }

// Verify recomputes the chain and reports the first inconsistency.
func (l *Log) Verify() error {
	prev := GenesisHash
	for i, e := range l.events {
		if e.Seq != uint64(i)+1 {
			return fmt.Errorf("%w: sequence at index %d", ErrChainBroken, i)
		}
		if e.PrevHash != prev {
			return fmt.Errorf("%w: predecessor at index %d", ErrChainBroken, i)
		}
		check := e
		check.Hash = ""
		if hashEvent(check) != e.Hash {
			return fmt.Errorf("%w: hash at index %d", ErrChainBroken, i)
		}
		prev = e.Hash
	}
	return nil
}

func hashEvent(e Event) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s|%s|%s",
		e.Seq,
		e.At.UTC().Format(time.RFC3339Nano),
		e.Actor, e.Action, e.Subject, e.Before, e.After,
		e.PrevHash,
	)
	return hex.EncodeToString(h.Sum(nil))
}
