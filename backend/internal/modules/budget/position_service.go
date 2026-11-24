package budget

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// PositionService implements the budget use cases. Every write is audited.
type PositionService struct {
	repo  PositionRepository
	audit AuditSink
}

// NewPositionService wires the service.
func NewPositionService(repo PositionRepository, sink AuditSink) *PositionService {
	return &PositionService{repo: repo, audit: sink}
}

// CreatePosition validates and stores a budget position.
func (s *PositionService) CreatePosition(ctx context.Context, year int, chapter, title, description string, allocatedCents int64, by string) (Position, error) {
	if strings.TrimSpace(by) == "" {
		return Position{}, ErrActorRequired
	}
	pos, err := NewPosition(year, chapter, title, description, allocatedCents)
	if err != nil {
		return Position{}, err
	}
	pos.CreatedBy = by
	saved, err := s.repo.CreatePosition(ctx, pos)
	if err != nil {
		return Position{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_position.create",
		Subject: positionSubject(saved.ID),
		After:   fmt.Sprintf("year=%d chapter=%s title=%s allocated_cents=%d", saved.Year, saved.Chapter, saved.Title, saved.AllocatedCents),
	}); err != nil {
		return Position{}, err
	}
	return saved, nil
}

// Positions returns the positions in budget order, one page at a time. It is read-only, so it is
// not audited.
func (s *PositionService) Positions(ctx context.Context, limit, offset int) ([]Position, error) {
	return s.repo.ListPositions(ctx, limit, offset)
}

// Post appends a posting, enforcing Mittelüberwachung.
//
// A commitment is bound once per reference: an approval retried after its order save lost a race
// must not bind the amount twice, so the second attempt returns the posting that is already there
// and is not audited again. A release or an actual is appended as it comes.
func (s *PositionService) Post(ctx context.Context, positionID int64, kind PostingKind, amountCents int64, reference, by string) (Posting, error) {
	if strings.TrimSpace(by) == "" {
		return Posting{}, ErrActorRequired
	}
	posting, err := NewPosting(positionID, kind, amountCents, reference)
	if err != nil {
		return Posting{}, err
	}
	posting.BookedBy = by
	var saved Posting
	inserted := true
	if kind == Commitment {
		// The repo holds the position's lock across the balance check and the insert, so two
		// approvals of the same reference cannot both bind it.
		saved, inserted, err = s.repo.BindCommitment(ctx, posting)
		if err != nil {
			return Posting{}, err
		}
		if !inserted {
			// The reference was already bound: the retry changed nothing, so it is not audited as
			// a second binding.
			return saved, nil
		}
	} else {
		bal, err := s.repo.Balance(ctx, positionID)
		if err != nil {
			return Posting{}, err
		}
		if err := CheckAvailable(bal, kind, amountCents); err != nil {
			return Posting{}, err
		}
		saved, err = s.repo.InsertPosting(ctx, posting)
		if err != nil {
			return Posting{}, err
		}
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_posting.create",
		Subject: positionSubject(positionID),
		After:   fmt.Sprintf("kind=%s amount_cents=%d reference=%s", saved.Kind, saved.AmountCents, saved.Reference),
	}); err != nil {
		return Posting{}, err
	}
	return saved, nil
}

// GetPosition loads a position.
func (s *PositionService) GetPosition(ctx context.Context, id int64) (Position, error) {
	return s.repo.GetPosition(ctx, id)
}

// Balance returns a position's derived balance.
func (s *PositionService) Balance(ctx context.Context, id int64) (Balance, error) {
	return s.repo.Balance(ctx, id)
}

func positionSubject(id int64) string { return fmt.Sprintf("budget_position/%d", id) }
