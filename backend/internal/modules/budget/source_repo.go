package budget

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Persistence for the sources and centres a position is assigned to (source.go).

// foreignKeyViolation is PostgreSQL's SQLSTATE for a foreign-key violation, which is what an
// unknown source or centre on an assignment raises.
const foreignKeyViolation = "23503"

const sourceColumns = `id, code, name, chapter, active, created_by`
const centerColumns = `id, code, name, active, created_by`

func scanSource(row pgx.Row) (Source, error) {
	var s Source
	err := row.Scan(&s.ID, &s.Code, &s.Name, &s.Chapter, &s.Active, &s.CreatedBy)
	return s, err
}

func scanCenter(row pgx.Row) (Center, error) {
	var c Center
	err := row.Scan(&c.ID, &c.Code, &c.Name, &c.Active, &c.CreatedBy)
	return c, err
}

// CreateSource stores a source. A code that is already taken is refused rather than merged: the
// code is how an import and a person name the same source, so two rows for one code would make
// that ambiguous.
func (r *PositionRepo) CreateSource(ctx context.Context, s Source) (Source, error) {
	saved, err := scanSource(r.pool.QueryRow(ctx,
		`INSERT INTO budget_source (code, name, chapter, active, created_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+sourceColumns,
		s.Code, s.Name, s.Chapter, s.Active, s.CreatedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Source{}, fmt.Errorf("%w: %s", ErrSourceExists, s.Code)
		}
		return Source{}, fmt.Errorf("budget: create source: %w", err)
	}
	return saved, nil
}

// Sources lists the sources by code, which is the order a screen and a report read them in.
func (r *PositionRepo) Sources(ctx context.Context) ([]Source, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+sourceColumns+` FROM budget_source ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("budget: list sources: %w", err)
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, fmt.Errorf("budget: scan source: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("budget: list sources: %w", err)
	}
	return out, nil
}

// CreateCenter stores a centre.
func (r *PositionRepo) CreateCenter(ctx context.Context, c Center) (Center, error) {
	saved, err := scanCenter(r.pool.QueryRow(ctx,
		`INSERT INTO budget_center (code, name, active, created_by)
		 VALUES ($1,$2,$3,$4) RETURNING `+centerColumns,
		c.Code, c.Name, c.Active, c.CreatedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Center{}, fmt.Errorf("%w: %s", ErrCenterExists, c.Code)
		}
		return Center{}, fmt.Errorf("budget: create centre: %w", err)
	}
	return saved, nil
}

// Centers lists the centres by code.
func (r *PositionRepo) Centers(ctx context.Context) ([]Center, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+centerColumns+` FROM budget_center ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("budget: list centres: %w", err)
	}
	defer rows.Close()
	out := []Center{}
	for rows.Next() {
		c, err := scanCenter(rows)
		if err != nil {
			return nil, fmt.Errorf("budget: scan centre: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("budget: list centres: %w", err)
	}
	return out, nil
}

// AssignPosition sets the source a position draws on and the centre that administers it. A nil
// clears that half, so naming one and clearing the other is one operation rather than two. An
// unknown source or centre is refused by the foreign keys and reported as the not-found it is,
// rather than as a generic failure.
func (r *PositionRepo) AssignPosition(ctx context.Context, positionID int64, sourceID, centerID *int64) (Position, error) {
	saved, err := scanPosition(r.pool.QueryRow(ctx,
		`UPDATE budget_position SET source_id = $2, center_id = $3 WHERE id = $1 RETURNING `+positionColumns,
		positionID, sourceID, centerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Position{}, fmt.Errorf("budget: position %d: %w", positionID, ErrPositionNotFound)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
			return Position{}, fmt.Errorf("%w: %s", ErrAssignmentUnknown, pgErr.ConstraintName)
		}
		return Position{}, fmt.Errorf("budget: assign position: %w", err)
	}
	return saved, nil
}
