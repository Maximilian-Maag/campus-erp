package audit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// auditLockKey serialises appends so the hash chain cannot fork.
const auditLockKey int64 = 0x4155444954 // "AUDIT"

// Store persists the audit stream in PostgreSQL (ADR-0006).
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Append seals an event onto the chain and persists it. Concurrent callers are
// serialised with an advisory lock taken for the duration of the transaction.
//
// The timestamp is truncated to microseconds and normalised to UTC: PostgreSQL's
// timestamptz stores microseconds, so hashing a nanosecond-precision time.Now()
// would not survive the round-trip and Verify would report a broken chain.
func (s *Store) Append(ctx context.Context, e Event) (Event, error) {
	e.At = e.At.UTC().Truncate(time.Microsecond)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("audit: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", auditLockKey); err != nil {
		return Event{}, fmt.Errorf("audit: lock: %w", err)
	}

	lastSeq := int64(0)
	lastHash := GenesisHash
	err = tx.QueryRow(ctx,
		"SELECT seq, hash FROM audit_event ORDER BY seq DESC LIMIT 1").Scan(&lastSeq, &lastHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Event{}, fmt.Errorf("audit: read head: %w", err)
	}

	e.Seq = uint64(lastSeq) + 1
	e.PrevHash = lastHash
	e.Hash = hashEvent(e)

	if _, err := tx.Exec(ctx,
		`INSERT INTO audit_event (seq, at, actor, action, subject, before, after, prev_hash, hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		int64(e.Seq), e.At, e.Actor, e.Action, e.Subject, e.Before, e.After, e.PrevHash, e.Hash,
	); err != nil {
		return Event{}, fmt.Errorf("audit: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("audit: commit: %w", err)
	}
	return e, nil
}

// LastEvent returns the most recent persisted event with the given action and
// subject, so a caller can look back at what it recorded previously (e.g. an ETL
// run comparing itself against its last run). It reports false when none exists.
func (s *Store) LastEvent(ctx context.Context, action, subject string) (Event, bool, error) {
	var e Event
	err := s.pool.QueryRow(ctx,
		`SELECT seq, at, actor, action, subject, before, after, prev_hash, hash
		 FROM audit_event WHERE action = $1 AND subject = $2
		 ORDER BY seq DESC LIMIT 1`, action, subject).
		Scan(&e.Seq, &e.At, &e.Actor, &e.Action, &e.Subject, &e.Before, &e.After, &e.PrevHash, &e.Hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, fmt.Errorf("audit: last event: %w", err)
	}
	return e, true, nil
}

// Bounds on a read of the audit stream, so a caller cannot ask for the whole stream by
// accident (or by passing a silly limit).
const (
	defaultEventLimit = 20
	maxEventLimit     = 200
)

// Events returns the most recent persisted events for an action, newest first. An empty
// subject matches every subject of that action, so a caller can read one subject's history
// or the action's across subjects. limit is clamped into [1, maxEventLimit].
//
// This is the read side of the same stream Append writes: what a run recorded is already
// audited, so reading it back needs no second store.
func (s *Store) Events(ctx context.Context, action, subject string, limit int) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT seq, at, actor, action, subject, before, after, prev_hash, hash
		 FROM audit_event WHERE action = $1 AND ($2 = '' OR subject = $2)
		 ORDER BY seq DESC LIMIT $3`, action, subject, clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("audit: events: %w", err)
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Seq, &e.At, &e.Actor, &e.Action, &e.Subject,
			&e.Before, &e.After, &e.PrevHash, &e.Hash); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: events: %w", err)
	}
	return out, nil
}

// clampLimit bounds a caller's limit into [1, maxEventLimit], defaulting a non-positive
// one, so a read cannot pull the whole stream by accident.
func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultEventLimit
	case limit > maxEventLimit:
		return maxEventLimit
	default:
		return limit
	}
}

// Verify recomputes the persisted chain and reports the first inconsistency.
func (s *Store) Verify(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		"SELECT seq, at, actor, action, subject, before, after, prev_hash, hash FROM audit_event ORDER BY seq")
	if err != nil {
		return fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	prev := GenesisHash
	i := 0
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Seq, &e.At, &e.Actor, &e.Action, &e.Subject,
			&e.Before, &e.After, &e.PrevHash, &e.Hash); err != nil {
			return fmt.Errorf("audit: scan: %w", err)
		}
		i++
		if e.Seq != uint64(i) {
			return fmt.Errorf("%w: sequence at %d", ErrChainBroken, i)
		}
		if e.PrevHash != prev {
			return fmt.Errorf("%w: predecessor at %d", ErrChainBroken, i)
		}
		check := e
		check.Hash = ""
		if hashEvent(check) != e.Hash {
			return fmt.Errorf("%w: hash at %d", ErrChainBroken, i)
		}
		prev = e.Hash
	}
	return rows.Err()
}
