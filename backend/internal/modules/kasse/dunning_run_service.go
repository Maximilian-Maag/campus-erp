package kasse

import (
	"context"
	"fmt"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// dunningLevelReader reads a year's dunning levels — the chain a run walks.
type dunningLevelReader interface {
	Levels(ctx context.Context, year int) ([]DunningLevel, error)
}

// dunningNoticeReader reads a claim's most recent notice. The bool is false when
// the claim has never been dunned.
type dunningNoticeReader interface {
	LatestNoticeByClaim(ctx context.Context, claimReference string) (DunningNotice, bool, error)
}

// openReceivableReader selects the open items a dunning run walks: those due on or
// before its as-of date.
type openReceivableReader interface {
	OpenReceivablesDueBy(ctx context.Context, asOf time.Time) ([]Receivable, error)
}

// dunningNoticeIssuer issues one notice — the dunning service itself, so a run's
// notices are rendered, stored and audited exactly like a hand-issued one.
type dunningNoticeIssuer interface {
	Issue(ctx context.Context, year int, levelCode, claimReference string, outstandingCents int64, by string) (DunningNotice, error)
}

// DunningRunService implements the dunning run (Mahnlauf, docs/28).
type DunningRunService struct {
	levels      dunningLevelReader
	notices     dunningNoticeReader
	receivables openReceivableReader
	issuer      dunningNoticeIssuer
	audit       AuditSink
}

// NewDunningRunService wires the run service. A single *DunningRepo satisfies both
// the level and the notice readers, so the caller normally passes it twice.
func NewDunningRunService(levels dunningLevelReader, notices dunningNoticeReader,
	receivables openReceivableReader, issuer dunningNoticeIssuer, sink AuditSink) *DunningRunService {

	return &DunningRunService{
		levels:      levels,
		notices:     notices,
		receivables: receivables,
		issuer:      issuer,
		audit:       sink,
	}
}

// Run advances each open receivable due by asOf one step along the year's dunning
// chain. A claim is dunned at most once per as-of date: a notice already issued on
// or after that date means the claim was dunned for this run and is skipped. The
// run reports every item it looked at, with the reason a skipped one was skipped;
// a failed issue does not abort the run, so one broken configuration or document
// does not hide the rest of the work. A read failure does abort it: an unreadable
// claim history would re-issue a step the claim already had.
func (s *DunningRunService) Run(ctx context.Context, year int, asOf time.Time, by string) (DunningRunResult, error) {
	if err := requireActor(by); err != nil {
		return DunningRunResult{}, err
	}
	if err := ValidateDunningRun(year, asOf); err != nil {
		return DunningRunResult{}, err
	}
	asOf = dateOnly(asOf)

	levels, err := s.levels.Levels(ctx, year)
	if err != nil {
		return DunningRunResult{}, err
	}
	chain, err := buildDunningChain(levels)
	if err != nil {
		return DunningRunResult{}, err
	}

	receivables, err := s.receivables.OpenReceivablesDueBy(ctx, asOf)
	if err != nil {
		return DunningRunResult{}, err
	}

	result := DunningRunResult{Year: year, AsOf: asOf, Outcomes: []DunningRunOutcome{}}
	for _, r := range receivables {
		outcome := DunningRunOutcome{
			ReceivableID:     r.ID,
			ClaimReference:   ReceivableClaimReference(r.ID),
			MandateReference: r.MandateReference,
			AmountCents:      r.AmountCents,
			DueDate:          dateOnly(r.DueDate),
		}
		next, skip, err := s.nextStep(ctx, chain, r, asOf)
		if err != nil {
			return DunningRunResult{}, err
		}
		outcome.LevelCode = next.Code
		if skip != "" {
			outcome.Reason = skip
			result.Skipped++
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		}

		notice, err := s.issuer.Issue(ctx, year, next.Code, outcome.ClaimReference, r.AmountCents, by)
		if err != nil {
			outcome.Reason = DunningSkipIssueFailed
			outcome.Detail = err.Error()
			result.Failed++
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		}
		outcome.Issued = true
		outcome.LevelCode = notice.LevelCode
		outcome.ChargeCents = notice.ChargeCents
		outcome.NoticeID = notice.ID
		outcome.DocumentKey = notice.DocumentKey
		result.Issued++
		result.Outcomes = append(result.Outcomes, outcome)
	}

	detail := fmt.Sprintf("year=%d as_of=%s issued=%d skipped=%d failed=%d",
		year, asOf.Format(time.DateOnly), result.Issued, result.Skipped, result.Failed)
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "dunning.run",
		Subject: fmt.Sprintf("dunning-run/%d/%s", year, asOf.Format(time.DateOnly)),
		After:   detail,
	}); err != nil {
		return DunningRunResult{}, err
	}
	return result, nil
}

// nextStep resolves the level a receivable should receive next. A non-empty skip
// reason means no notice is due: the claim was already dunned for this run, has no
// next level, names a level the year does not carry, or its grace period has not
// elapsed.
func (s *DunningRunService) nextStep(ctx context.Context, chain dunningChain, r Receivable,
	asOf time.Time) (DunningLevel, string, error) {

	latest, found, err := s.notices.LatestNoticeByClaim(ctx, ReceivableClaimReference(r.ID))
	if err != nil {
		return DunningLevel{}, "", err
	}
	if !found {
		head := chain.head
		return head, dueSkip(head, dateOnly(r.DueDate), asOf), nil
	}
	if !dateOnly(latest.IssuedOn).Before(asOf) {
		return DunningLevel{}, DunningSkipAlreadyDunned, nil
	}
	level, ok := chain.levels[latest.LevelCode]
	if !ok {
		return DunningLevel{}, DunningSkipUnknownLevel, nil
	}
	if level.NextCode == "" {
		return DunningLevel{}, DunningSkipNoNextLevel, nil
	}
	next, ok := chain.levels[level.NextCode]
	if !ok {
		return DunningLevel{}, DunningSkipNextLevelMissing, nil
	}
	return next, dueSkip(next, dateOnly(latest.IssuedOn), asOf), nil
}

// dueSkip reports whether a level's grace period counted from a step date has not
// elapsed by asOf. A grace period of zero days is due the same day.
func dueSkip(level DunningLevel, previousStep, asOf time.Time) string {
	if previousStep.AddDate(0, 0, level.DaysAllowed).After(asOf) {
		return DunningSkipNotDue
	}
	return ""
}
