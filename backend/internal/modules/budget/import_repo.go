package budget

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
)

// ImportResult counts what an import did. It is the answer an operator gets, so it distinguishes a
// creation from a find: "12 angelegt, 3 schon vorhanden" is a different statement from "15
// angelegt", and only one of them is true on a second run of the same file.
type ImportResult struct {
	// Layout is the layout the file was read with, so the audit says what was interpreted.
	Layout string
	// SourcesCreated / SourcesKept count the Mittelquellen; kept means the file named a code that
	// already existed, whose name is left as it is.
	SourcesCreated int
	SourcesKept    int
	// CentersCreated / CentersKept count the Bewirtschaftungsstellen, on the same rule.
	CentersCreated int
	CentersKept    int
	// PositionsCreated / PositionsKept count the Haushaltsstellen; kept means the year, chapter and
	// title were already there, whose Ansatz and description are left as they are.
	PositionsCreated int
	PositionsKept    int
	// PositionsNamed counts the positions this import named — the ones it gave a source or a centre
	// they did not have. An assignment a person made is never overwritten, so a re-import of a
	// corrected file cannot undo a correction.
	PositionsNamed int
}

// ImportBudget writes a parsed budget file in **one transaction**: the sources and centres the file
// names, the positions it carries, and the naming of each position from its own line. It creates
// what is missing and leaves what is there, which is what makes a second run of the same file a
// no-op counted as kept rather than a duplicate.
func (r *PositionRepo) ImportBudget(ctx context.Context, rows []budgetimport.Row, layout, by string) (ImportResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ImportResult{}, fmt.Errorf("budget: begin the import: %w", err)
	}
	// A rollback after a commit is a no-op; anything else leaves the budget as it was.
	defer func() { _ = tx.Rollback(ctx) }()

	res := ImportResult{Layout: layout}
	for _, row := range rows {
		var sourceID, centerID *int64
		if row.SourceCode != "" {
			id, created, err := upsertSource(ctx, tx, row, by)
			if err != nil {
				return ImportResult{}, err
			}
			sourceID = &id
			if created {
				res.SourcesCreated++
			} else {
				res.SourcesKept++
			}
		}
		if row.CenterCode != "" {
			id, created, err := upsertCenter(ctx, tx, row, by)
			if err != nil {
				return ImportResult{}, err
			}
			centerID = &id
			if created {
				res.CentersCreated++
			} else {
				res.CentersKept++
			}
		}
		positionID, created, err := upsertPosition(ctx, tx, row, by)
		if err != nil {
			return ImportResult{}, err
		}
		if created {
			res.PositionsCreated++
		} else {
			res.PositionsKept++
		}
		if sourceID == nil && centerID == nil {
			continue
		}
		named, err := nameUnnamed(ctx, tx, positionID, sourceID, centerID)
		if err != nil {
			return ImportResult{}, err
		}
		if named {
			res.PositionsNamed++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportResult{}, fmt.Errorf("budget: commit the import: %w", err)
	}
	return res, nil
}

// upsertSource creates a Mittelquelle unless its code is already taken, and returns its id either
// way. The name and the chapter are used **only when it is created**: a name a person may have
// corrected is not silently overwritten by the next import of an older file.
func upsertSource(ctx context.Context, tx pgx.Tx, row budgetimport.Row, by string) (int64, bool, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO budget_source (code, name, chapter, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code) DO NOTHING
		RETURNING id`,
		row.SourceCode, row.SourceName, row.SourceChapter, by).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("budget: insert the source: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM budget_source WHERE code = $1`, row.SourceCode).Scan(&id); err != nil {
		return 0, false, fmt.Errorf("budget: read the source by code: %w", err)
	}
	return id, false, nil
}

// upsertCenter is upsertSource for a Bewirtschaftungsstelle, which has no chapter.
func upsertCenter(ctx context.Context, tx pgx.Tx, row budgetimport.Row, by string) (int64, bool, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO budget_center (code, name, created_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (code) DO NOTHING
		RETURNING id`,
		row.CenterCode, row.CenterName, by).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("budget: insert the centre: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM budget_center WHERE code = $1`, row.CenterCode).Scan(&id); err != nil {
		return 0, false, fmt.Errorf("budget: read the centre by code: %w", err)
	}
	return id, false, nil
}

// upsertPosition creates the position unless the year, chapter and title are already there. The
// Ansatz and the description are creation-only, on the same rule as the names above.
func upsertPosition(ctx context.Context, tx pgx.Tx, row budgetimport.Row, by string) (int64, bool, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO budget_position (year, chapter, title, description, allocated_cents, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (year, chapter, title, cost_centre, cost_carrier) DO NOTHING
		RETURNING id`,
		row.Year, row.Chapter, row.Title, row.Description, row.AllocatedCents, by).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("budget: insert the position: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT id FROM budget_position WHERE year = $1 AND chapter = $2 AND title = $3`,
		row.Year, row.Chapter, row.Title).Scan(&id); err != nil {
		return 0, false, fmt.Errorf("budget: read the position: %w", err)
	}
	return id, false, nil
}

// nameUnnamed gives a position a half it does not have, and never replaces one it does: COALESCE
// keeps what is there. It reports whether it named anything.
func nameUnnamed(ctx context.Context, tx pgx.Tx, positionID int64, sourceID, centerID *int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE budget_position
		SET source_id = COALESCE(source_id, $2), center_id = COALESCE(center_id, $3)
		WHERE id = $1
		  AND ((source_id IS NULL AND $2::bigint IS NOT NULL) OR (center_id IS NULL AND $3::bigint IS NOT NULL))`,
		positionID, sourceID, centerID)
	if err != nil {
		return false, fmt.Errorf("budget: name the position: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
