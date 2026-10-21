package kasse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is PostgreSQL's SQLSTATE for a unique-constraint violation.
const uniqueViolation = "23505"

// Repo is the PostgreSQL implementation of Repository.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo returns a Repo backed by pool.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const dayColumns = "id, business_date, status, opening_cents, closing_cents"

func scanDay(row pgx.Row) (Day, error) {
	var d Day
	err := row.Scan(&d.ID, &d.BusinessDate, &d.Status, &d.OpeningCents, &d.ClosingCents)
	return d, err
}

// OpenDay inserts a new open day.
func (r *Repo) OpenDay(ctx context.Context, date time.Time, openingCents int64, by string) (Day, error) {
	d, err := scanDay(r.pool.QueryRow(ctx,
		`INSERT INTO cash_book_day (business_date, status, opening_cents, opened_by)
		 VALUES ($1, 'open', $2, $3) RETURNING `+dayColumns,
		date, openingCents, by))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Day{}, fmt.Errorf("kasse: day %s: %w", date.Format(time.DateOnly), ErrDayExists)
		}
		return Day{}, fmt.Errorf("kasse: open day: %w", err)
	}
	return d, nil
}

// GetDay loads a day by id.
func (r *Repo) GetDay(ctx context.Context, id int64) (Day, error) {
	d, err := scanDay(r.pool.QueryRow(ctx,
		`SELECT `+dayColumns+` FROM cash_book_day WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Day{}, fmt.Errorf("kasse: day %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Day{}, fmt.Errorf("kasse: get day: %w", err)
	}
	return d, nil
}

// Bookings returns a day's bookings in insertion order.
func (r *Repo) Bookings(ctx context.Context, dayID int64) ([]Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, day_id, kind, amount_cents, purpose FROM cash_booking WHERE day_id = $1 ORDER BY id`, dayID)
	if err != nil {
		return nil, fmt.Errorf("kasse: bookings: %w", err)
	}
	defer rows.Close()
	var out []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.DayID, &b.Kind, &b.AmountCents, &b.Purpose); err != nil {
			return nil, fmt.Errorf("kasse: scan booking: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// InsertBooking persists a booking.
func (r *Repo) InsertBooking(ctx context.Context, b Booking, by string) (Booking, error) {
	var out Booking
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cash_booking (day_id, kind, amount_cents, purpose, booked_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, day_id, kind, amount_cents, purpose`,
		b.DayID, b.Kind, b.AmountCents, b.Purpose, by,
	).Scan(&out.ID, &out.DayID, &out.Kind, &out.AmountCents, &out.Purpose)
	if err != nil {
		return Booking{}, fmt.Errorf("kasse: insert booking: %w", err)
	}
	return out, nil
}

// CloseDay finalises an open day with its computed closing balance.
func (r *Repo) CloseDay(ctx context.Context, id, closingCents int64, by string) (Day, error) {
	d, err := scanDay(r.pool.QueryRow(ctx,
		`UPDATE cash_book_day
		    SET status = 'closed', closing_cents = $2, closed_by = $3, closed_at = now()
		  WHERE id = $1 AND status = 'open'
		  RETURNING `+dayColumns, id, closingCents, by))
	if errors.Is(err, pgx.ErrNoRows) {
		return Day{}, ErrDayClosed
	}
	if err != nil {
		return Day{}, fmt.Errorf("kasse: close day: %w", err)
	}
	return d, nil
}
