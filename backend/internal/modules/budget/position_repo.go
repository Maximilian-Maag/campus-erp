package budget

import (
	"context"

	"errors"
	"fmt"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"

// PositionRepository is the persistence port for budget positions.
type PositionRepository interface {
	CreatePosition(ctx context.Context, p Position) (Position, error)
	GetPosition(ctx context.Context, id int64) (Position, error)
	// ListPositions returns every position in budget order, for the Mittelübersicht (docs/25).
	ListPositions(ctx context.Context) ([]Position, error)
	InsertPosting(ctx context.Context, p Posting) (Posting, error)
	Postings(ctx context.Context, positionID int64) ([]Posting, error)
	Balance(ctx context.Context, positionID int64) (Balance, error)
	// CreateSource, Sources, CreateCenter and Centers are the Mittelquellen and
	// Bewirtschaftungsstellen (source.go).
	CreateSource(ctx context.Context, s Source) (Source, error)
	Sources(ctx context.Context) ([]Source, error)
	CreateCenter(ctx context.Context, c Center) (Center, error)
	Centers(ctx context.Context) ([]Center, error)
	// AssignPosition sets which source a position draws on and which centre administers it.
	AssignPosition(ctx context.Context, positionID int64, sourceID, centerID *int64) (Position, error)
	// ImportBudget writes a parsed budget file in one transaction — the sources and centres it
	// names, the positions it carries, and each position's naming from its own line (import_repo.go).
	ImportBudget(ctx context.Context, rows []budgetimport.Row, layout, by string) (ImportResult, error)
}

// PositionRepo is the PostgreSQL implementation.
type PositionRepo struct{ pool *pgxpool.Pool }

// NewPositionRepo returns a PositionRepo.
func NewPositionRepo(pool *pgxpool.Pool) *PositionRepo { return &PositionRepo{pool: pool} }

const positionColumns = `id, year, chapter, title, description, allocated_cents, created_by, source_id, center_id`
const postingColumns = `id, position_id, kind, amount_cents, reference, booked_by`

func scanPosition(row pgx.Row) (Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.Year, &p.Chapter, &p.Title, &p.Description, &p.AllocatedCents, &p.CreatedBy,
		&p.SourceID, &p.CenterID)
	return p, err
}

func scanPosting(row pgx.Row) (Posting, error) {
	var p Posting
	err := row.Scan(&p.ID, &p.PositionID, &p.Kind, &p.AmountCents, &p.Reference, &p.BookedBy)
	return p, err
}

// CreatePosition stores a budget position.
func (r *PositionRepo) CreatePosition(ctx context.Context, p Position) (Position, error) {
	saved, err := scanPosition(r.pool.QueryRow(ctx,
		`INSERT INTO budget_position (year, chapter, title, description, allocated_cents, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+positionColumns,
		p.Year, p.Chapter, p.Title, p.Description, p.AllocatedCents, p.CreatedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Position{}, ErrPositionExists
		}
		return Position{}, fmt.Errorf("budget: create position: %w", err)
	}
	return saved, nil
}

// GetPosition loads a position by id.
// ListPositions returns every position, ordered the way a budget is read: year, then chapter,
// then title.
func (r *PositionRepo) ListPositions(ctx context.Context) ([]Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+positionColumns+` FROM budget_position ORDER BY year, chapter, title, id`)
	if err != nil {
		return nil, fmt.Errorf("budget: list positions: %w", err)
	}
	defer rows.Close()
	out := []Position{}
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, fmt.Errorf("budget: scan position: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("budget: list positions: %w", err)
	}
	return out, nil
}

func (r *PositionRepo) GetPosition(ctx context.Context, id int64) (Position, error) {
	p, err := scanPosition(r.pool.QueryRow(ctx, `SELECT `+positionColumns+` FROM budget_position WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Position{}, fmt.Errorf("budget: position %d: %w", id, ErrPositionNotFound)
	}
	if err != nil {
		return Position{}, fmt.Errorf("budget: get position: %w", err)
	}
	return p, nil
}

// InsertPosting appends a posting.
func (r *PositionRepo) InsertPosting(ctx context.Context, p Posting) (Posting, error) {
	saved, err := scanPosting(r.pool.QueryRow(ctx,
		`INSERT INTO budget_posting (position_id, kind, amount_cents, reference, booked_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+postingColumns,
		p.PositionID, p.Kind, p.AmountCents, p.Reference, p.BookedBy))
	if err != nil {
		return Posting{}, fmt.Errorf("budget: insert posting: %w", err)
	}
	return saved, nil
}

// Postings lists a position's postings, oldest first.
func (r *PositionRepo) Postings(ctx context.Context, positionID int64) ([]Posting, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+postingColumns+` FROM budget_posting WHERE position_id = $1 ORDER BY id`, positionID)
	if err != nil {
		return nil, fmt.Errorf("budget: postings: %w", err)
	}
	defer rows.Close()
	var out []Posting
	for rows.Next() {
		p, err := scanPosting(rows)
		if err != nil {
			return nil, fmt.Errorf("budget: scan posting: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Balance returns the derived state of a position (allocated, committed, actual).
func (r *PositionRepo) Balance(ctx context.Context, positionID int64) (Balance, error) {
	pos, err := r.GetPosition(ctx, positionID)
	if err != nil {
		return Balance{}, err
	}
	var committed, actual int64
	err = r.pool.QueryRow(ctx,
		`SELECT coalesce(sum(amount_cents) FILTER (WHERE kind = 'commitment'), 0)
		      - coalesce(sum(amount_cents) FILTER (WHERE kind = 'release'), 0),
		        coalesce(sum(amount_cents) FILTER (WHERE kind = 'actual'), 0)
		   FROM budget_posting WHERE position_id = $1`, positionID).Scan(&committed, &actual)
	if err != nil {
		return Balance{}, fmt.Errorf("budget: balance: %w", err)
	}
	return Balance{
		PositionID:     positionID,
		AllocatedCents: pos.AllocatedCents,
		CommittedCents: committed,
		ActualCents:    actual,
	}, nil
}
