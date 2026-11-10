package budget

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// The use cases around the sources and centres a position is assigned to (source.go). Every write
// is audited like the rest of the budget; the reads are not.

// CreateSource validates and stores a Mittelquelle.
func (s *PositionService) CreateSource(ctx context.Context, code, name, chapter, by string) (Source, error) {
	if strings.TrimSpace(by) == "" {
		return Source{}, ErrActorRequired
	}
	src, err := NewSource(code, name, chapter)
	if err != nil {
		return Source{}, err
	}
	src.CreatedBy = by
	saved, err := s.repo.CreateSource(ctx, src)
	if err != nil {
		return Source{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_source.create",
		Subject: sourceSubject(saved.ID),
		After:   fmt.Sprintf("code=%s name=%s chapter=%s", saved.Code, saved.Name, saved.Chapter),
	}); err != nil {
		return Source{}, err
	}
	return saved, nil
}

// Sources lists the Mittelquellen in code order. It is read-only, so it is not audited.
func (s *PositionService) Sources(ctx context.Context) ([]Source, error) {
	return s.repo.Sources(ctx)
}

// CreateCenter validates and stores a Bewirtschaftungsstelle.
func (s *PositionService) CreateCenter(ctx context.Context, code, name, by string) (Center, error) {
	if strings.TrimSpace(by) == "" {
		return Center{}, ErrActorRequired
	}
	center, err := NewCenter(code, name)
	if err != nil {
		return Center{}, err
	}
	center.CreatedBy = by
	saved, err := s.repo.CreateCenter(ctx, center)
	if err != nil {
		return Center{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_center.create",
		Subject: centerSubject(saved.ID),
		After:   fmt.Sprintf("code=%s name=%s", saved.Code, saved.Name),
	}); err != nil {
		return Center{}, err
	}
	return saved, nil
}

// Centers lists the Bewirtschaftungsstellen in code order.
func (s *PositionService) Centers(ctx context.Context) ([]Center, error) {
	return s.repo.Centers(ctx)
}

// AssignPosition names the source a position draws on and the centre that administers it. A nil
// half clears it, so naming one and clearing the other is one operation rather than two. The
// position is read first, so a position nothing carries is a not-found *before* the foreign keys
// are asked about the source and the centre.
func (s *PositionService) AssignPosition(ctx context.Context, positionID int64, sourceID, centerID *int64, by string) (Position, error) {
	if strings.TrimSpace(by) == "" {
		return Position{}, ErrActorRequired
	}
	if _, err := s.repo.GetPosition(ctx, positionID); err != nil {
		return Position{}, err
	}
	saved, err := s.repo.AssignPosition(ctx, positionID, sourceID, centerID)
	if err != nil {
		return Position{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_position.assign",
		Subject: positionSubject(saved.ID),
		After:   fmt.Sprintf("source_id=%s center_id=%s", idOrNone(saved.SourceID), idOrNone(saved.CenterID)),
	}); err != nil {
		return Position{}, err
	}
	return saved, nil
}

// idOrNone renders a nullable id for an audit detail, so "cleared" is visible rather than absent.
func idOrNone(id *int64) string {
	if id == nil {
		return "none"
	}
	return fmt.Sprintf("%d", *id)
}

func sourceSubject(id int64) string { return fmt.Sprintf("budget_source/%d", id) }
func centerSubject(id int64) string { return fmt.Sprintf("budget_center/%d", id) }
