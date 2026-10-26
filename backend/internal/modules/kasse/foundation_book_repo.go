package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foreignKeyViolation is PostgreSQL's FK-violation SQLSTATE.
const foreignKeyViolation = "23503"

// FoundationBookRepository is the persistence port for foundation books.
type FoundationBookRepository interface {
	CreateBook(ctx context.Context, b FoundationBook) (FoundationBook, error)
	GetBook(ctx context.Context, id int64) (FoundationBook, error)
	InsertBooking(ctx context.Context, b FoundationBooking, by string) (FoundationBooking, error)
	Bookings(ctx context.Context, bookID int64) ([]FoundationBooking, error)
	Balance(ctx context.Context, bookID int64) (FoundationBalance, error)
}

// FoundationBookRepo is the PostgreSQL implementation.
type FoundationBookRepo struct{ pool *pgxpool.Pool }

// NewFoundationBookRepo returns a FoundationBookRepo.
func NewFoundationBookRepo(pool *pgxpool.Pool) *FoundationBookRepo {
	return &FoundationBookRepo{pool: pool}
}

const bookColumns = `id, foundation_id, reference, currency, created_by`

const foundationBookingColumns = `id, book_id, kind, amount_cents, purpose, booked_by`

func scanBook(row pgx.Row) (FoundationBook, error) {
	var b FoundationBook
	err := row.Scan(&b.ID, &b.FoundationID, &b.Reference, &b.Currency, &b.CreatedBy)
	return b, err
}

func scanFoundationBooking(row pgx.Row) (FoundationBooking, error) {
	var b FoundationBooking
	err := row.Scan(&b.ID, &b.BookID, &b.Kind, &b.AmountCents, &b.Purpose, &b.BookedBy)
	return b, err
}

// CreateBook stores a foundation book.
func (r *FoundationBookRepo) CreateBook(ctx context.Context, b FoundationBook) (FoundationBook, error) {
	saved, err := scanBook(r.pool.QueryRow(ctx,
		`INSERT INTO foundation_book (foundation_id, reference, currency, created_by)
		 VALUES ($1,$2,$3,$4)
		 RETURNING `+bookColumns,
		b.FoundationID, b.Reference, b.Currency, b.CreatedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case uniqueViolation:
				return FoundationBook{}, ErrFoundationBookExists
			case foreignKeyViolation:
				return FoundationBook{}, fmt.Errorf("kasse: foundation %d: %w", b.FoundationID, ErrNotFound)
			}
		}
		return FoundationBook{}, fmt.Errorf("kasse: create foundation book: %w", err)
	}
	return saved, nil
}

// GetBook loads a book by id.
func (r *FoundationBookRepo) GetBook(ctx context.Context, id int64) (FoundationBook, error) {
	b, err := scanBook(r.pool.QueryRow(ctx, `SELECT `+bookColumns+` FROM foundation_book WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return FoundationBook{}, fmt.Errorf("kasse: foundation book %d: %w", id, ErrFoundationBookNotFound)
	}
	if err != nil {
		return FoundationBook{}, fmt.Errorf("kasse: get foundation book: %w", err)
	}
	return b, nil
}

// InsertBooking appends a movement.
func (r *FoundationBookRepo) InsertBooking(ctx context.Context, b FoundationBooking, by string) (FoundationBooking, error) {
	saved, err := scanFoundationBooking(r.pool.QueryRow(ctx,
		`INSERT INTO foundation_booking (book_id, kind, amount_cents, purpose, booked_by)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING `+foundationBookingColumns,
		b.BookID, b.Kind, b.AmountCents, b.Purpose, by))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
			return FoundationBooking{}, fmt.Errorf("kasse: book %d: %w", b.BookID, ErrFoundationBookNotFound)
		}
		return FoundationBooking{}, fmt.Errorf("kasse: insert foundation booking: %w", err)
	}
	return saved, nil
}

// Bookings lists a book's movements, oldest first. It is scoped to the book id,
// so one foundation's movements never appear in another's.
func (r *FoundationBookRepo) Bookings(ctx context.Context, bookID int64) ([]FoundationBooking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+foundationBookingColumns+` FROM foundation_booking WHERE book_id = $1 ORDER BY id`, bookID)
	if err != nil {
		return nil, fmt.Errorf("kasse: foundation bookings: %w", err)
	}
	defer rows.Close()
	var out []FoundationBooking
	for rows.Next() {
		b, err := scanFoundationBooking(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan foundation booking: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Balance sums a book's movements, scoped to the book id.
func (r *FoundationBookRepo) Balance(ctx context.Context, bookID int64) (FoundationBalance, error) {
	var bal FoundationBalance
	bal.BookID = bookID
	err := r.pool.QueryRow(ctx,
		`SELECT coalesce(sum(amount_cents) FILTER (WHERE kind = 'income'), 0),
		        coalesce(sum(amount_cents) FILTER (WHERE kind = 'expense'), 0)
		   FROM foundation_booking WHERE book_id = $1`, bookID).Scan(&bal.IncomeCents, &bal.ExpenseCents)
	if err != nil {
		return FoundationBalance{}, fmt.Errorf("kasse: foundation balance: %w", err)
	}
	return bal, nil
}
