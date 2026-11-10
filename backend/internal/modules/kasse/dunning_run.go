package kasse

import (
	"errors"
	"fmt"
	"time"
)

// The dunning run (Mahnlauf) walks the open receivables and advances each claim
// one step along its dunning chain: the level a claim has reached is read from its
// most recent notice, and the next level is the one that notice's level names in
// `next_code` (docs/28). A claim that has never been dunned starts at the chain's
// first level. The run issues the next level only when its grace period
// (`days_allowed`, counted from the previous step) has elapsed, and only once a
// day per claim — so re-running the same day is a no-op rather than a second
// notice.
//
// The chain is configuration: a level names its successor by code. Exactly one
// level must be the first (no other level's `next_code` points at it); a `next_code`
// that names no level of the year ends the chain there rather than failing the run,
// because a level may be configured before its successor exists. Anything else —
// no first level, several, or a cycle — is refused rather than guessed at.

var (
	// ErrRunYearInvalid is returned when a dunning run's year is outside 1900..2999.
	ErrRunYearInvalid = errors.New("kasse: a dunning run's year must be between 1900 and 2999")
	// ErrRunDateRequired is returned when a dunning run has no as-of date.
	ErrRunDateRequired = errors.New("kasse: a dunning run needs an as-of date")
	// ErrDunningChainHead is returned when the year's levels do not have exactly one
	// first level, so there is no unambiguous place for a claim to start.
	ErrDunningChainHead = errors.New("kasse: the dunning levels must form one chain with exactly one first level")
	// ErrDunningChainBroken is returned when the levels contain a cycle or a level the
	// first level cannot reach.
	ErrDunningChainBroken = errors.New("kasse: the dunning chain is broken (a cycle or an unreachable level)")
)

// Why a claim did not advance in a run. An issued claim carries no reason.
const (
	// DunningSkipAlreadyDunned: the claim already carries a notice issued on or after
	// the run's as-of date, so a second run the same day must not dun it again.
	DunningSkipAlreadyDunned = "already_dunned"
	// DunningSkipNotDue: the next level's grace period has not elapsed yet.
	DunningSkipNotDue = "not_due"
	// DunningSkipUnknownLevel: the claim's latest notice names a level that this
	// year's chain does not carry (e.g. a retired configuration).
	DunningSkipUnknownLevel = "unknown_level"
	// DunningSkipNoNextLevel: the claim's latest level has no successor — the chain
	// ends there, so there is nothing further to issue.
	DunningSkipNoNextLevel = "no_next_level"
	// DunningSkipNextLevelMissing: the latest level names a successor the year does
	// not carry, so the run cannot know what to issue.
	DunningSkipNextLevelMissing = "next_level_missing"
	// DunningSkipIssueFailed: issuing the next notice failed; the detail carries the
	// reason. The run continues with the other claims rather than aborting.
	DunningSkipIssueFailed = "issue_failed"
)

// DunningRunOutcome is what became of one open receivable in a run: the notice it
// received, or why it received none. It is flat rather than nesting the receivable
// and the notice so the screen can render the run's report without joining.
type DunningRunOutcome struct {
	ReceivableID     int64
	ClaimReference   string
	MandateReference string
	AmountCents      int64
	DueDate          time.Time
	LevelCode        string
	ChargeCents      int64
	NoticeID         int64
	DocumentKey      string
	Issued           bool
	Reason           string
	Detail           string
}

// DunningRunResult is the report of one run: the as-of date, the counts, and one
// outcome per receivable the run looked at.
type DunningRunResult struct {
	Year     int
	AsOf     time.Time
	Issued   int
	Skipped  int
	Failed   int
	Outcomes []DunningRunOutcome
}

// ReceivableClaimReference is the claim reference a run stores on the notices it
// issues for an open receivable. The receivable's id is stable and unique, so the
// notice stays traceable to the item without a second identifier on the item; the
// form mirrors the audit subject convention (`receivable/<id>`).
func ReceivableClaimReference(id int64) string { return fmt.Sprintf("receivable/%d", id) }

// ValidateDunningRun checks the run's own inputs. The as-of date is a date, not a
// moment: a run's grace periods are counted in days.
func ValidateDunningRun(year int, asOf time.Time) error {
	if year < 1900 || year > 2999 {
		return ErrRunYearInvalid
	}
	if asOf.IsZero() {
		return ErrRunDateRequired
	}
	return nil
}

// dunningChain is a year's levels resolved into one chain: a lookup by code and the
// single level a never-dunned claim starts at.
type dunningChain struct {
	levels map[string]DunningLevel
	head   DunningLevel
}

// buildDunningChain resolves the year's levels into one chain. It refuses a
// configuration it cannot walk unambiguously rather than picking a level: a missing
// first level or several, a cycle, or a level the first one cannot reach. A
// `next_code` naming no existing level is *not* an error — it ends the chain, which
// lets a level be configured before its successor.
func buildDunningChain(levels []DunningLevel) (dunningChain, error) {
	if len(levels) == 0 {
		return dunningChain{}, ErrDunningChainHead
	}
	byCode := make(map[string]DunningLevel, len(levels))
	for _, l := range levels {
		byCode[l.Code] = l
	}
	referenced := make(map[string]bool, len(levels))
	for _, l := range levels {
		if l.NextCode != "" {
			referenced[l.NextCode] = true
		}
	}
	heads := make([]DunningLevel, 0, 1)
	for _, l := range levels {
		if !referenced[l.Code] {
			heads = append(heads, l)
		}
	}
	if len(heads) != 1 {
		return dunningChain{}, ErrDunningChainHead
	}

	seen := make(map[string]bool, len(levels))
	cur := heads[0]
	for {
		if seen[cur.Code] {
			return dunningChain{}, ErrDunningChainBroken
		}
		seen[cur.Code] = true
		next, ok := byCode[cur.NextCode]
		if cur.NextCode == "" || !ok {
			break
		}
		cur = next
	}
	if len(seen) != len(levels) {
		// A component the head cannot reach — a cycle elsewhere, or a level whose
		// only predecessor is itself in a loop.
		return dunningChain{}, ErrDunningChainBroken
	}
	return dunningChain{levels: byCode, head: heads[0]}, nil
}

// dateOnly reduces a moment to its date in UTC. Dunning arithmetic is in days and
// the stores keep dates, so the time of day must not decide whether a step is due.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
