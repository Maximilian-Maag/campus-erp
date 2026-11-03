package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderRepository is the persistence port for payment orders.
type OrderRepository interface {
	CreateRecipient(ctx context.Context, r Recipient) (Recipient, error)
	CreateBudgetPosition(ctx context.Context, b BudgetPosition) (BudgetPosition, error)
	InsertOrder(ctx context.Context, o PaymentOrder) (PaymentOrder, error)
	GetOrder(ctx context.Context, id int64) (PaymentOrder, error)
	SaveOrderStatus(ctx context.Context, o PaymentOrder, from OrderStatus) (PaymentOrder, error)
	RecordDecision(ctx context.Context, orderID int64, decision OrderStatus, actor, reason string) error
}

// OrderRepo is the PostgreSQL implementation.
type OrderRepo struct{ pool *pgxpool.Pool }

// NewOrderRepo returns an OrderRepo.
func NewOrderRepo(pool *pgxpool.Pool) *OrderRepo { return &OrderRepo{pool: pool} }

const orderColumns = `id, reference, business_year, amount_cents, currency, recipient_id,
	budget_position_id, purpose, due_date, status, created_by,
	coalesce(submitted_by, ''), coalesce(decided_by, ''), coalesce(decision_reason, '')`

func scanOrder(row pgx.Row) (PaymentOrder, error) {
	var o PaymentOrder
	err := row.Scan(&o.ID, &o.Reference, &o.BusinessYear, &o.AmountCents, &o.Currency,
		&o.RecipientID, &o.BudgetPositionID, &o.Purpose, &o.DueDate, &o.Status, &o.CreatedBy,
		&o.SubmittedBy, &o.DecidedBy, &o.DecisionReason)
	return o, err
}

// CreateRecipient upserts a recipient keyed by IBAN and returns the stored row.
func (r *OrderRepo) CreateRecipient(ctx context.Context, rec Recipient) (Recipient, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO payment_recipient (name, account_holder, iban, bic, bank_name)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (iban) DO UPDATE
		   SET name = EXCLUDED.name, account_holder = EXCLUDED.account_holder,
		       bic = EXCLUDED.bic, bank_name = EXCLUDED.bank_name
		 RETURNING id, name, account_holder, iban, bic, bank_name`,
		rec.Name, rec.AccountHolder, rec.IBAN, rec.BIC, rec.BankName)
	var out Recipient
	if err := row.Scan(&out.ID, &out.Name, &out.AccountHolder, &out.IBAN, &out.BIC, &out.BankName); err != nil {
		return Recipient{}, fmt.Errorf("kasse: create recipient: %w", err)
	}
	return out, nil
}

// CreateBudgetPosition upserts a budget position and returns the stored row.
func (r *OrderRepo) CreateBudgetPosition(ctx context.Context, b BudgetPosition) (BudgetPosition, error) {
	// The allocation is written on insert only: the ON CONFLICT update deliberately leaves
	// allocated_cents alone, so naming an existing position cannot rewrite its Ansatz.
	row := r.pool.QueryRow(ctx,
		`INSERT INTO budget_position (year, chapter, title, cost_centre, cost_carrier, allocated_cents)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (year, chapter, title, cost_centre, cost_carrier) DO UPDATE
		   SET year = EXCLUDED.year
		 RETURNING id, year, chapter, title, cost_centre, cost_carrier, allocated_cents`,
		b.Year, b.Chapter, b.Title, b.CostCentre, b.CostCarrier, b.AllocatedCents)
	var out BudgetPosition
	if err := row.Scan(&out.ID, &out.Year, &out.Chapter, &out.Title, &out.CostCentre, &out.CostCarrier,
		&out.AllocatedCents); err != nil {
		return BudgetPosition{}, fmt.Errorf("kasse: create budget position: %w", err)
	}
	return out, nil
}

// InsertOrder stores a new draft order.
func (r *OrderRepo) InsertOrder(ctx context.Context, o PaymentOrder) (PaymentOrder, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO payment_order
		   (reference, business_year, amount_cents, currency, recipient_id,
		    budget_position_id, purpose, due_date, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING `+orderColumns,
		o.Reference, o.BusinessYear, o.AmountCents, o.Currency, o.RecipientID,
		o.BudgetPositionID, o.Purpose, o.DueDate, o.Status, o.CreatedBy)
	saved, err := scanOrder(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return PaymentOrder{}, fmt.Errorf("kasse: reference %q: %w", o.Reference, ErrReferenceExists)
		}
		return PaymentOrder{}, fmt.Errorf("kasse: insert order: %w", err)
	}
	return saved, nil
}

// GetOrder loads an order by id.
func (r *OrderRepo) GetOrder(ctx context.Context, id int64) (PaymentOrder, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM payment_order WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOrder{}, fmt.Errorf("kasse: order %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("kasse: get order: %w", err)
	}
	return o, nil
}

// SaveOrderStatus writes a transition, guarded by the status it was read at
// (optimistic locking). A zero-row update means someone else moved the order.
func (r *OrderRepo) SaveOrderStatus(ctx context.Context, o PaymentOrder, from OrderStatus) (PaymentOrder, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE payment_order
		    SET status = $2,
		        submitted_by = nullif($3, ''),
		        submitted_at = CASE WHEN $3 <> '' THEN now() ELSE submitted_at END,
		        decided_by = nullif($4, ''),
		        decided_at = CASE WHEN $4 <> '' THEN now() ELSE decided_at END,
		        decision_reason = $5
		  WHERE id = $1 AND status = $6
		  RETURNING `+orderColumns,
		o.ID, o.Status, o.SubmittedBy, o.DecidedBy, o.DecisionReason, from)
	saved, err := scanOrder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentOrder{}, ErrOrderConflict
	}
	if err != nil {
		return PaymentOrder{}, fmt.Errorf("kasse: save order status: %w", err)
	}
	return saved, nil
}

// RecordDecision appends to the decision trail.
func (r *OrderRepo) RecordDecision(ctx context.Context, orderID int64, decision OrderStatus, actor, reason string) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO payment_order_decision (order_id, decision, actor, reason) VALUES ($1,$2,$3,$4)`,
		orderID, decision, actor, reason); err != nil {
		return fmt.Errorf("kasse: record decision: %w", err)
	}
	return nil
}
