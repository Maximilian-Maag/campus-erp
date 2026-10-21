package kasse

import (
	"context"
	"fmt"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// DunningService implements the dunning workflow (S5).
type DunningService struct {
	repo  DunningRepository
	audit AuditSink
}

// NewDunningService wires the dunning service.
func NewDunningService(repo DunningRepository, sink AuditSink) *DunningService {
	return &DunningService{repo: repo, audit: sink}
}

// CreateLevel validates and stores a dunning level with its bands.
func (s *DunningService) CreateLevel(ctx context.Context, l DunningLevel, by string) (DunningLevel, error) {
	if err := l.Validate(); err != nil {
		return DunningLevel{}, err
	}
	if err := requireActor(by); err != nil {
		return DunningLevel{}, err
	}
	saved, err := s.repo.CreateLevel(ctx, l)
	if err != nil {
		return DunningLevel{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "dunning.level.create",
		Subject: dunningLevelSubject(saved.Year, saved.Code),
		After:   fmt.Sprintf("bands=%d days_allowed=%d active=%t", len(saved.Bands), saved.DaysAllowed, saved.Active),
	}); err != nil {
		return DunningLevel{}, err
	}
	return saved, nil
}

// Level loads a dunning level.
func (s *DunningService) Level(ctx context.Context, year int, code string) (DunningLevel, error) {
	return s.repo.GetLevel(ctx, year, code)
}

// Issue computes the charge for a claim and records a dunning notice.
func (s *DunningService) Issue(ctx context.Context, year int, levelCode, claimReference string,
	outstandingCents int64, by string) (DunningNotice, error) {

	level, err := s.repo.GetLevel(ctx, year, levelCode)
	if err != nil {
		return DunningNotice{}, err
	}
	notice, err := NewDunningNotice(level, claimReference, outstandingCents, nowFunc(), by)
	if err != nil {
		return DunningNotice{}, err
	}
	saved, err := s.repo.InsertNotice(ctx, notice)
	if err != nil {
		return DunningNotice{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "dunning.notice.issue",
		Subject: fmt.Sprintf("notice/%d", saved.ID),
		After:   fmt.Sprintf("claim=%s level=%s outstanding_cents=%d charge_cents=%d", saved.ClaimReference, saved.LevelCode, saved.OutstandingCents, saved.ChargeCents),
	}); err != nil {
		return DunningNotice{}, err
	}
	return saved, nil
}

func dunningLevelSubject(year int, code string) string {
	return fmt.Sprintf("dunning-level/%d/%s", year, code)
}
