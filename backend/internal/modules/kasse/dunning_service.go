package kasse

import (
	"context"
	"fmt"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// DunningDocuments renders a notice to a document and stores it, returning the key of
// the artefact (docs/24). A service without one issues notices that carry no document.
type DunningDocuments interface {
	StoreNotice(ctx context.Context, n DunningNotice, level DunningLevel) (string, error)
}

// DunningService implements the dunning workflow (S5).
type DunningService struct {
	repo  DunningRepository
	audit AuditSink
	docs  DunningDocuments
}

// NewDunningService wires the dunning service.
func NewDunningService(repo DunningRepository, sink AuditSink) *DunningService {
	return &DunningService{repo: repo, audit: sink}
}

// WithDocuments makes the service produce a document for every notice it issues.
func (s *DunningService) WithDocuments(d DunningDocuments) *DunningService {
	s.docs = d
	return s
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

// Notice loads a notice by id, including the key of its stored document.
func (s *DunningService) Notice(ctx context.Context, id int64) (DunningNotice, error) {
	return s.repo.GetNotice(ctx, id)
}

// Level loads a dunning level.
func (s *DunningService) Level(ctx context.Context, year int, code string) (DunningLevel, error) {
	return s.repo.GetLevel(ctx, year, code)
}

// Levels lists the dunning levels of one year, or of every year when year is zero —
// what a screen needs to offer the level a notice is issued against.
func (s *DunningService) Levels(ctx context.Context, year int) ([]DunningLevel, error) {
	return s.repo.Levels(ctx, year)
}

// Issue computes the charge for a claim and records a dunning notice, stamped with the date the
// issue is made as of. A run passes its own as-of date, so the grace period of the next step counts
// from the day the step belongs to rather than from the day the run happened to execute.
func (s *DunningService) Issue(ctx context.Context, year int, levelCode, receivableReference string,
	outstandingCents int64, issuedOn time.Time, by string) (DunningNotice, error) {

	level, err := s.repo.GetLevel(ctx, year, levelCode)
	if err != nil {
		return DunningNotice{}, err
	}
	notice, err := NewDunningNotice(level, receivableReference, outstandingCents, issuedOn, by)
	if err != nil {
		return DunningNotice{}, err
	}
	saved, err := s.repo.InsertNotice(ctx, notice)
	if err != nil {
		return DunningNotice{}, err
	}
	// Render and store the document, and record where it went on the notice. A failure
	// here fails the issue: a notice that claims to have a document must have one.
	if s.docs != nil {
		key, err := s.docs.StoreNotice(ctx, saved, level)
		if err != nil {
			return DunningNotice{}, err
		}
		if err := s.repo.SetNoticeDocument(ctx, saved.ID, key); err != nil {
			return DunningNotice{}, err
		}
		saved.DocumentKey = key
	}
	detail := fmt.Sprintf("claim=%s level=%s outstanding_cents=%d charge_cents=%d",
		saved.ReceivableReference, saved.LevelCode, saved.OutstandingCents, saved.ChargeCents)
	if saved.DocumentKey != "" {
		detail += " document=" + saved.DocumentKey
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "dunning.notice.issue",
		Subject: fmt.Sprintf("notice/%d", saved.ID),
		After:   detail,
	}); err != nil {
		return DunningNotice{}, err
	}
	return saved, nil
}

func dunningLevelSubject(year int, code string) string {
	return fmt.Sprintf("dunning-level/%d/%s", year, code)
}
