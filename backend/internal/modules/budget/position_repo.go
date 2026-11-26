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
	ListPositions(ctx context.Context, limit, offset int) ([]Position, error)
	InsertPosting(ctx context.Context, p Posting) (Posting, error)
	// BindCommitment stores a commitment once per reference: a retried approval of the same
	// economic event ends up with one posting rather than two. The bool reports whether the row
	// was inserted. The position row is locked across the balance check and the insert, so a
	// concurrent binding cannot over-commit the position.
	BindCommitment(ctx context.Context, p Posting) (Posting, bool, error)
	Postings(ctx context.Context, positionID int64) ([]Posting, error)
	Balance(ctx context.Context, positionID int64) (Balance, error)
	// CreateSource, Sources, CreateCentre and Centres are the Mittelquellen and
	// Bewirtschaftungsstellen (source.go).
	CreateSource(ctx context.Context, s Source) (Source, error)
	Sources(ctx context.Context) ([]Source, error)
	CreateCentre(ctx context.Context, c Centre) (Centre, error)
	Centres(ctx context.Context) ([]Centre, error)
	// AssignPosition sets which source a position draws on and which centre administers it.
	AssignPosition(ctx context.Context, positionID int64, sourceID, centreID *int64) (Position, error)
	// ImportBudget writes a parsed budget file in one transaction — the sources and centres it
	// names, the positions it carries, and each position's naming from its own line (import_repo.go).
	ImportBudget(ctx context.Context, rows []budgetimport.Row, layout, by string) (ImportResult, error)
}

// PositionRepo is the PostgreSQL implementation.
type PositionRepo struct{ pool *pgxpool.Pool }

// NewPositionRepo returns a PositionRepo.
func NewPositionRepo(pool *pgxpool.Pool) *PositionRepo { return &PositionRepo{pool: pool} }

const positionColumns = `id, year, chapter, title, description, allocated_cents, created_by, source_id, centre_id`
const postingColumns = `id, position_id, kind, amount_cents, reference, booked_by`

func scanPosition(row pgx.Row) (Position, error) {
	var p Position
	err := row.Scan(&p.ID, &p.Year, &p.Chapter, &p.Title, &p.Description, &p.AllocatedCents, &p.CreatedBy,
		&p.SourceID, &p.CentreID)
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
// ListPositions returns the positions, ordered the way a budget is read — year, then chapter, then
// title — one page at a time.
//
// The order ends in id, the primary key, so it is total: two positions can share a year, a chapter
// and a title, and an order that did would repeat one row at a page boundary and skip another.
func (r *PositionRepo) ListPositions(ctx context.Context, limit, offset int) ([]Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+positionColumns+` FROM budget_position
		  ORDER BY year, chapter, title, id LIMIT $1 OFFSET $2`, limit, offset)
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

// BindCommitment appends a commitment unless the reference is already bound on the position.
//
// A payment order's approval binds its amount and then saves the order. If the save loses a race
// the order stays un-approved while its amount stays bound, and the operator's retry would bind it
// a second time. Binding by reference makes the retry a no-op: the reference names the economic
// event, so a second commitment for it would be a duplicate, not a second event. A release for the
// same reference cancels the binding, so a reference whose commitments and releases net to zero is
// bound again on the next attempt.
//
// The position row is locked for the whole check-and-insert: two concurrent approvals of the same
// reference cannot both pass the "already bound" test, and the availability check is made against
// the balance the lock protects.
func (r *PositionRepo) BindCommitment(ctx context.Context, p Posting) (Posting, bool, error) {
	if p.Kind != Commitment {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %s is not a commitment", p.Kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the position first: every binding on it is serialised, so the balance read below cannot
	// be stale by the time the row is inserted.
	var allocated int64
	err = tx.QueryRow(ctx,
		`SELECT allocated_cents FROM budget_position WHERE id = $1 FOR UPDATE`, p.PositionID).Scan(&allocated)
	if errors.Is(err, pgx.ErrNoRows) {
		return Posting{}, false, fmt.Errorf("budget: position %d: %w", p.PositionID, ErrPositionNotFound)
	}
	if err != nil {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
	}

	var bound int64
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE kind = 'commitment') - count(*) FILTER (WHERE kind = 'release')
		   FROM budget_posting WHERE position_id = $1 AND reference = $2`,
		p.PositionID, p.Reference).Scan(&bound); err != nil {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
	}
	if bound > 0 {
		existing, err := scanPosting(tx.QueryRow(ctx,
			`SELECT `+postingColumns+` FROM budget_posting
			  WHERE position_id = $1 AND kind = 'commitment' AND reference = $2
			  ORDER BY id DESC LIMIT 1`, p.PositionID, p.Reference))
		if err != nil {
			return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
		}
		return existing, false, nil
	}

	var committed, actual int64
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(sum(amount_cents) FILTER (WHERE kind = 'commitment'), 0)
		      - coalesce(sum(amount_cents) FILTER (WHERE kind = 'release'), 0),
		        coalesce(sum(amount_cents) FILTER (WHERE kind = 'actual'), 0)
		   FROM budget_posting WHERE position_id = $1`, p.PositionID).Scan(&committed, &actual); err != nil {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
	}
	if err := CheckAvailable(Balance{PositionID: p.PositionID, AllocatedCents: allocated,
		CommittedCents: committed, ActualCents: actual}, p.Kind, p.AmountCents); err != nil {
		return Posting{}, false, err
	}

	saved, err := scanPosting(tx.QueryRow(ctx,
		`INSERT INTO budget_posting (position_id, kind, amount_cents, reference, booked_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+postingColumns,
		p.PositionID, p.Kind, p.AmountCents, p.Reference, p.BookedBy))
	if err != nil {
		return Posting{}, false, fmt.Errorf("budget: insert posting: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Posting{}, false, fmt.Errorf("budget: bind commitment: %w", err)
	}
	return saved, true, nil
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
