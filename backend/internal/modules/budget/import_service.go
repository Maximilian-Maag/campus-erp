package budget

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
)

// Import reads a budget file — Mittelquellen, Bewirtschaftungsstellen and Haushaltsstellen in one
// pass — and writes it (docs/10).
//
// The file is read **whole before anything is written**: a file with one unreadable line imports
// nothing and says which line is unreadable, because a half-imported budget is worse than none.
// The layout is data (`budgetimport.Layout`); an unknown layout id is refused rather than guessed.
func (s *PositionService) Import(ctx context.Context, layoutID, content, by string) (ImportResult, error) {
	if strings.TrimSpace(by) == "" {
		return ImportResult{}, ErrActorRequired
	}
	layout, err := budgetimport.LayoutByID(layoutID)
	if err != nil {
		return ImportResult{}, err
	}
	rows, err := budgetimport.Parse(layout, content)
	if err != nil {
		return ImportResult{}, err
	}
	res, err := s.repo.ImportBudget(ctx, rows, layout.ID, by)
	if err != nil {
		return ImportResult{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "budget_import.run",
		Subject: "budget_import/" + layout.ID,
		After: fmt.Sprintf(
			"sources_created=%d sources_kept=%d centers_created=%d centers_kept=%d positions_created=%d positions_kept=%d positions_named=%d",
			res.SourcesCreated, res.SourcesKept, res.CentersCreated, res.CentersKept,
			res.PositionsCreated, res.PositionsKept, res.PositionsNamed),
	}); err != nil {
		return ImportResult{}, err
	}
	return res, nil
}
