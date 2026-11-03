package procurement

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"

// OrderRepository is the persistence port for purchase orders.
type OrderRepository interface {
	Insert(ctx context.Context, o Order) (Order, error)
	Get(ctx context.Context, id int64) (Order, error)
	// GetByNumber is how an inbound invoice finds its order: the buyer reference the document
	// carries is the order number (BT-10).
	GetByNumber(ctx context.Context, number string) (Order, error)
	// List returns the order summaries, optionally narrowed to one status ("" for every status).
	List(ctx context.Context, status Status) ([]OrderSummary, error)
	SetStatus(ctx context.Context, id int64, status Status, by string) (Order, error)
}

// OrderRepo is the PostgreSQL implementation.
type OrderRepo struct{ pool *pgxpool.Pool }

// NewOrderRepo returns an OrderRepo.
func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo { return &OrderRepo{pool: pool} }

const orderColumns = `id, number, supplier, budget_position_id, currency,
	net_cents, vat_cents, gross_cents, status, created_by`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.Number, &o.Supplier, &o.BudgetPositionID, &o.Currency,
		&o.NetCents, &o.VatCents, &o.GrossCents, &o.Status, &o.CreatedBy)
	return o, err
}

// Insert stores an order with its lines atomically.
func (r *OrderRepo) Insert(ctx context.Context, o Order) (Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO purchase_order
		   (number, supplier, budget_position_id, currency, net_cents, vat_cents, gross_cents, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		o.Number, o.Supplier, o.BudgetPositionID, o.Currency, o.NetCents, o.VatCents, o.GrossCents, o.Status, o.CreatedBy).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Order{}, ErrOrderExists
		}
		return Order{}, fmt.Errorf("procurement: insert order: %w", err)
	}
	for _, l := range o.Lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO purchase_order_line
			   (order_id, line_no, description, quantity_milli, unit_price_cents, net_cents, vat_category, vat_rate_bp)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, l.LineNo, l.Description, l.QuantityMilli, l.UnitPriceCents, l.NetCents, l.VatCategory, l.VatRateBP); err != nil {
			return Order{}, fmt.Errorf("procurement: insert line: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("procurement: commit: %w", err)
	}
	o.ID = id
	return o, nil
}

// List returns the orders as summaries, ordered by number. An empty status lists every status.
func (r *OrderRepo) List(ctx context.Context, status Status) ([]OrderSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, number, supplier, budget_position_id, currency, net_cents, vat_cents, gross_cents, status
		   FROM purchase_order
		  WHERE ($1 = '' OR status = $1)
		  ORDER BY number`, string(status))
	if err != nil {
		return nil, fmt.Errorf("procurement: list orders: %w", err)
	}
	defer rows.Close()
	out := []OrderSummary{}
	for rows.Next() {
		var s OrderSummary
		if err := rows.Scan(&s.ID, &s.Number, &s.Supplier, &s.BudgetPositionID, &s.Currency,
			&s.NetCents, &s.VatCents, &s.GrossCents, &s.Status); err != nil {
			return nil, fmt.Errorf("procurement: scan order summary: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("procurement: list orders: %w", err)
	}
	return out, nil
}

// GetByNumber loads an order by its number. It resolves the id and reuses Get, so the line
// loading exists once.
func (r *OrderRepo) GetByNumber(ctx context.Context, number string) (Order, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM purchase_order WHERE number = $1`, number).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("procurement: order %q: %w", number, ErrOrderNotFound)
	}
	if err != nil {
		return Order{}, fmt.Errorf("procurement: get order by number: %w", err)
	}
	return r.Get(ctx, id)
}

// Get loads an order with its lines.
func (r *OrderRepo) Get(ctx context.Context, id int64) (Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM purchase_order WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("procurement: order %d: %w", id, ErrOrderNotFound)
	}
	if err != nil {
		return Order{}, fmt.Errorf("procurement: get order: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT line_no, description, quantity_milli, unit_price_cents, net_cents, vat_category, vat_rate_bp
		   FROM purchase_order_line WHERE order_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return Order{}, fmt.Errorf("procurement: get lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.LineNo, &l.Description, &l.QuantityMilli, &l.UnitPriceCents,
			&l.NetCents, &l.VatCategory, &l.VatRateBP); err != nil {
			return Order{}, fmt.Errorf("procurement: scan line: %w", err)
		}
		o.Lines = append(o.Lines, l)
	}
	return o, rows.Err()
}

// SetStatus updates the lifecycle status (and the matching audit stamp).
func (r *OrderRepo) SetStatus(ctx context.Context, id int64, status Status, by string) (Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx,
		`UPDATE purchase_order
		    SET status = $2,
		        approved_by = CASE WHEN $2 = 'approved' THEN $3 ELSE approved_by END,
		        approved_at = CASE WHEN $2 = 'approved' THEN now()   ELSE approved_at END,
		        received_by = CASE WHEN $2 = 'received' THEN $3 ELSE received_by END,
		        received_at = CASE WHEN $2 = 'received' THEN now()   ELSE received_at END
		  WHERE id = $1
		 RETURNING `+orderColumns, id, status, by))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("procurement: order %d: %w", id, ErrOrderNotFound)
	}
	if err != nil {
		return Order{}, fmt.Errorf("procurement: set status: %w", err)
	}
	return o, nil
}
