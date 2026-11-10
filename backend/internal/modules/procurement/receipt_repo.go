package procurement

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Goods-receipt persistence (docs/12). It hangs off OrderRepo because a receipt only exists
// relative to its order.

const receiptColumns = `id, order_id, reference, received_cents, received_on, received_by`

func scanReceipt(row pgx.Row) (Receipt, error) {
	var r Receipt
	var receivedOn time.Time
	err := row.Scan(&r.ID, &r.OrderID, &r.Reference, &r.ReceivedCents, &receivedOn, &r.ReceivedBy)
	if err != nil {
		return Receipt{}, err
	}
	r.ReceivedOn = receivedOn.UTC()
	return r, nil
}

// InsertReceipt stores a receipt with its lines atomically. A delivery note already recorded for
// the order is refused (the unique constraint is the idempotency key), so a retried delivery
// cannot be counted twice.
func (r *OrderRepo) InsertReceipt(ctx context.Context, receipt Receipt) (Receipt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Receipt{}, fmt.Errorf("procurement: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	saved, err := scanReceipt(tx.QueryRow(ctx,
		`INSERT INTO goods_receipt (order_id, reference, received_cents, received_on, received_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+receiptColumns,
		receipt.OrderID, receipt.Reference, receipt.ReceivedCents, receipt.ReceivedOn, receipt.ReceivedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return Receipt{}, fmt.Errorf("%w: %s", ErrReceiptExists, receipt.Reference)
		}
		return Receipt{}, fmt.Errorf("procurement: insert receipt: %w", err)
	}
	for _, l := range receipt.Lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO goods_receipt_line (receipt_id, line_no, quantity_milli) VALUES ($1,$2,$3)`,
			saved.ID, l.LineNo, l.QuantityMilli); err != nil {
			return Receipt{}, fmt.Errorf("procurement: insert receipt line: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Receipt{}, fmt.Errorf("procurement: commit: %w", err)
	}
	saved.Lines = receipt.Lines
	return saved, nil
}

// Receipts lists an order's receipts with their lines, oldest first.
func (r *OrderRepo) Receipts(ctx context.Context, orderID int64) ([]Receipt, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+receiptColumns+` FROM goods_receipt WHERE order_id = $1 ORDER BY received_on, id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("procurement: list receipts: %w", err)
	}
	defer rows.Close()
	out := []Receipt{}
	for rows.Next() {
		receipt, err := scanReceipt(rows)
		if err != nil {
			return nil, fmt.Errorf("procurement: scan receipt: %w", err)
		}
		out = append(out, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("procurement: list receipts: %w", err)
	}
	for i := range out {
		lines, err := r.receiptLines(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Lines = lines
	}
	return out, nil
}

func (r *OrderRepo) receiptLines(ctx context.Context, receiptID int64) ([]ReceiptLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT line_no, quantity_milli FROM goods_receipt_line WHERE receipt_id = $1 ORDER BY line_no`, receiptID)
	if err != nil {
		return nil, fmt.Errorf("procurement: receipt lines: %w", err)
	}
	defer rows.Close()
	out := []ReceiptLine{}
	for rows.Next() {
		var l ReceiptLine
		if err := rows.Scan(&l.LineNo, &l.QuantityMilli); err != nil {
			return nil, fmt.Errorf("procurement: scan receipt line: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("procurement: receipt lines: %w", err)
	}
	return out, nil
}

// GoodsReceived aggregates an order's receipts: the quantity received per line number, and the
// value they have converted. An order with no receipt is zero of both rather than an error — a
// delivery is optional, and the first one needs the same answer as the tenth.
func (r *OrderRepo) GoodsReceived(ctx context.Context, orderID int64) (map[int]int64, int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.line_no, sum(l.quantity_milli)
		   FROM goods_receipt_line l
		   JOIN goods_receipt r ON r.id = l.receipt_id
		  WHERE r.order_id = $1
		  GROUP BY l.line_no
		  ORDER BY l.line_no`, orderID)
	if err != nil {
		return nil, 0, fmt.Errorf("procurement: goods received: %w", err)
	}
	defer rows.Close()
	received := map[int]int64{}
	for rows.Next() {
		var lineNo, qty int64
		if err := rows.Scan(&lineNo, &qty); err != nil {
			return nil, 0, fmt.Errorf("procurement: scan goods received: %w", err)
		}
		received[int(lineNo)] = qty
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("procurement: goods received: %w", err)
	}

	var converted int64
	if err := r.pool.QueryRow(ctx,
		`SELECT coalesce(sum(received_cents), 0) FROM goods_receipt WHERE order_id = $1`,
		orderID).Scan(&converted); err != nil {
		return nil, 0, fmt.Errorf("procurement: converted value: %w", err)
	}
	return received, converted, nil
}
