package kasse

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LedgerBooking is one confirmed booking as the posting export needs it.
type LedgerBooking struct {
	// Reference is the export's idempotency key: the import reference when the
	// booking carries one, a stable derived reference otherwise.
	Reference    string
	BusinessDate time.Time
	Kind         BookingKind
	AmountCents  int64
	Purpose      string
}

// LedgerRepository lists the bookings a posting export covers.
type LedgerRepository interface {
	ListBookingsForExport(ctx context.Context, from, to time.Time) ([]LedgerBooking, error)
}

// LedgerRepo reads bookings for the posting export.
type LedgerRepo struct{ pool *pgxpool.Pool }

// NewLedgerRepo returns a LedgerRepo backed by pool.
func NewLedgerRepo(pool *pgxpool.Pool) *LedgerRepo { return &LedgerRepo{pool: pool} }

// ListBookingsForExport returns every booking made on a day in [from, to], in
// (business date, booking id) order so the collection is deterministic.
func (r *LedgerRepo) ListBookingsForExport(ctx context.Context, from, to time.Time) ([]LedgerBooking, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT coalesce(nullif(b.legacy_ref, ''), 'BK-' || lpad(b.id::text, 6, '0')),
		       d.business_date, b.kind, b.amount_cents, b.purpose
		  FROM cash_booking b
		  JOIN cash_book_day d ON d.id = b.day_id
		 WHERE d.business_date BETWEEN $1 AND $2
		 ORDER BY d.business_date, b.id`, from, to)
	if err != nil {
		return nil, fmt.Errorf("kasse: list bookings for export: %w", err)
	}
	defer rows.Close()
	out := []LedgerBooking{}
	for rows.Next() {
		var b LedgerBooking
		if err := rows.Scan(&b.Reference, &b.BusinessDate, &b.Kind, &b.AmountCents, &b.Purpose); err != nil {
			return nil, fmt.Errorf("kasse: scan booking: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: list bookings for export: %w", err)
	}
	return out, nil
}
