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
	// BookByReference finds a foundation's book by its reference, so a carry-over opens the book it
	// already made instead of failing on the unique (foundation, reference) pair.
	BookByReference(ctx context.Context, foundationID int64, reference string) (FoundationBook, bool, error)
	InsertBooking(ctx context.Context, b FoundationBooking, by string) (FoundationBooking, error)
	Bookings(ctx context.Context, bookID int64) ([]FoundationBooking, error)
	Books(ctx context.Context, foundationID int64) ([]FoundationBook, error)
	Balance(ctx context.Context, bookID int64) (FoundationBalance, error)
	// FoundationByID loads the foundation a book belongs to: the carry-over needs its year, its
	// number and whether it may carry at all.
	FoundationByID(ctx context.Context, id int64) (Foundation, error)
	// FoundationByYearNumber finds one year's foundation record — the one a carry-over opens.
	FoundationByYearNumber(ctx context.Context, year int, number string) (Foundation, bool, error)
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

// Books lists a foundation's books, oldest first. It is scoped to the foundation id, so one
// foundation's books never appear among another's.
func (r *FoundationBookRepo) Books(ctx context.Context, foundationID int64) ([]FoundationBook, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+bookColumns+` FROM foundation_book WHERE foundation_id = $1 ORDER BY id`, foundationID)
	if err != nil {
		return nil, fmt.Errorf("kasse: foundation books: %w", err)
	}
	defer rows.Close()
	out := []FoundationBook{}
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan foundation book: %w", err)
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

// foundationReadColumns is the foundation reference as the book side reads it. The tax repo owns the
// write path for foundation master data; this repo reads it because a book and the year it belongs to
// are one question at carry-over time.
const foundationReadColumns = `id, year, number, description, carry_over`

func scanFoundationRead(row pgx.Row) (Foundation, error) {
	var f Foundation
	err := row.Scan(&f.ID, &f.Year, &f.Number, &f.Description, &f.CarryOver)
	return f, err
}

// BookByReference finds a foundation's book by its reference.
func (r *FoundationBookRepo) BookByReference(ctx context.Context, foundationID int64, reference string) (FoundationBook, bool, error) {
	b, err := scanBook(r.pool.QueryRow(ctx,
		`SELECT `+bookColumns+` FROM foundation_book WHERE foundation_id = $1 AND reference = $2`,
		foundationID, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return FoundationBook{}, false, nil
	}
	if err != nil {
		return FoundationBook{}, false, fmt.Errorf("kasse: find foundation book: %w", err)
	}
	return b, true, nil
}

// FoundationByID loads the foundation a book belongs to.
func (r *FoundationBookRepo) FoundationByID(ctx context.Context, id int64) (Foundation, error) {
	f, err := scanFoundationRead(r.pool.QueryRow(ctx,
		`SELECT `+foundationReadColumns+` FROM foundation WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Foundation{}, fmt.Errorf("kasse: foundation %d: %w", id, ErrFoundationRequired)
	}
	if err != nil {
		return Foundation{}, fmt.Errorf("kasse: get foundation: %w", err)
	}
	return f, nil
}

// FoundationByYearNumber finds one year's foundation record. A missing one is not an error: the
// caller decides what to say about a year that has not been opened yet.
func (r *FoundationBookRepo) FoundationByYearNumber(ctx context.Context, year int, number string) (Foundation, bool, error) {
	f, err := scanFoundationRead(r.pool.QueryRow(ctx,
		`SELECT `+foundationReadColumns+` FROM foundation WHERE year = $1 AND number = $2`, year, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return Foundation{}, false, nil
	}
	if err != nil {
		return Foundation{}, false, fmt.Errorf("kasse: find foundation: %w", err)
	}
	return f, true, nil
}
