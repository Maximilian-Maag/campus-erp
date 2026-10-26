package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PaymentRunRepository is the persistence port for payment runs.
type PaymentRunRepository interface {
	CreateCashAccount(ctx context.Context, a CashAccount) (CashAccount, error)
	GetCashAccount(ctx context.Context, id int64) (CashAccount, error)
	RunByReference(ctx context.Context, reference string) (PaymentRun, bool, error)
	GetRun(ctx context.Context, id int64) (PaymentRun, error)
	RunFile(ctx context.Context, id int64) (string, error)
	LoadPayableOrders(ctx context.Context, orderIDs []int64) ([]PayableOrder, error)
	InsertRun(ctx context.Context, run PaymentRun, fileXML string, items []PaymentRunItem) (PaymentRun, error)
}

// PaymentRunRepo is the PostgreSQL implementation.
type PaymentRunRepo struct{ pool *pgxpool.Pool }

// NewPaymentRunRepo returns a PaymentRunRepo.
func NewPaymentRunRepo(pool *pgxpool.Pool) *PaymentRunRepo { return &PaymentRunRepo{pool: pool} }

const runColumns = `id, reference, account_id, execution_date, message_id,
	item_count, sum_cents, file_hash, created_by`

func scanRun(row pgx.Row) (PaymentRun, error) {
	var r PaymentRun
	err := row.Scan(&r.ID, &r.Reference, &r.AccountID, &r.ExecutionDate, &r.MessageID,
		&r.ItemCount, &r.SumCents, &r.FileHash, &r.CreatedBy)
	return r, err
}

// CreateCashAccount upserts a cash account keyed by IBAN.
func (r *PaymentRunRepo) CreateCashAccount(ctx context.Context, a CashAccount) (CashAccount, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO cash_account (name, iban, bic, currency)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (iban) DO UPDATE
		   SET name = EXCLUDED.name, bic = EXCLUDED.bic, currency = EXCLUDED.currency
		 RETURNING id, name, iban, bic, currency`,
		a.Name, a.IBAN, a.BIC, a.Currency)
	var out CashAccount
	if err := row.Scan(&out.ID, &out.Name, &out.IBAN, &out.BIC, &out.Currency); err != nil {
		return CashAccount{}, fmt.Errorf("kasse: create cash account: %w", err)
	}
	return out, nil
}

// GetCashAccount loads an account by id.
func (r *PaymentRunRepo) GetCashAccount(ctx context.Context, id int64) (CashAccount, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, name, iban, bic, currency FROM cash_account WHERE id = $1`, id)
	var out CashAccount
	if err := row.Scan(&out.ID, &out.Name, &out.IBAN, &out.BIC, &out.Currency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CashAccount{}, fmt.Errorf("kasse: cash account %d: %w", id, ErrNotFound)
		}
		return CashAccount{}, fmt.Errorf("kasse: get cash account: %w", err)
	}
	return out, nil
}

// RunByReference returns the run with the given reference, if any.
func (r *PaymentRunRepo) RunByReference(ctx context.Context, reference string) (PaymentRun, bool, error) {
	run, err := scanRun(r.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM payment_run WHERE reference = $1`, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentRun{}, false, nil
	}
	if err != nil {
		return PaymentRun{}, false, fmt.Errorf("kasse: run by reference: %w", err)
	}
	return run, true, nil
}

// GetRun loads a run by id.
func (r *PaymentRunRepo) GetRun(ctx context.Context, id int64) (PaymentRun, error) {
	run, err := scanRun(r.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM payment_run WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentRun{}, fmt.Errorf("kasse: run %d: %w", id, ErrRunNotFound)
	}
	if err != nil {
		return PaymentRun{}, fmt.Errorf("kasse: get run: %w", err)
	}
	return run, nil
}

// RunFile returns the stored bank file of a run.
func (r *PaymentRunRepo) RunFile(ctx context.Context, id int64) (string, error) {
	var xml string
	err := r.pool.QueryRow(ctx, `SELECT file_xml FROM payment_run WHERE id = $1`, id).Scan(&xml)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("kasse: run %d: %w", id, ErrRunNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("kasse: get run file: %w", err)
	}
	return xml, nil
}

// LoadPayableOrders loads the given orders joined with their recipient. Orders
// that do not exist are simply absent from the result; the caller checks the
// count.
func (r *PaymentRunRepo) LoadPayableOrders(ctx context.Context, orderIDs []int64) ([]PayableOrder, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT o.id, o.reference, o.amount_cents, o.currency, o.purpose, o.status,
		        p.account_holder, p.iban, p.bic
		   FROM payment_order o
		   JOIN payment_recipient p ON p.id = o.recipient_id
		  WHERE o.id = ANY($1)
		  ORDER BY o.id`, orderIDs)
	if err != nil {
		return nil, fmt.Errorf("kasse: load payable orders: %w", err)
	}
	defer rows.Close()
	var out []PayableOrder
	for rows.Next() {
		var o PayableOrder
		if err := rows.Scan(&o.ID, &o.Reference, &o.AmountCents, &o.Currency, &o.Purpose,
			&o.Status, &o.CreditorName, &o.CreditorIBAN, &o.CreditorBIC); err != nil {
			return nil, fmt.Errorf("kasse: scan payable order: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// InsertRun stores a run and its items atomically. A duplicate reference maps to
// ErrRunReferenceExists; an order already in a run maps to ErrOrderAlreadyPaid.
func (r *PaymentRunRepo) InsertRun(ctx context.Context, run PaymentRun, fileXML string, items []PaymentRunItem) (PaymentRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return PaymentRun{}, fmt.Errorf("kasse: begin run: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := scanRun(tx.QueryRow(ctx,
		`INSERT INTO payment_run
		   (reference, account_id, execution_date, message_id, item_count, sum_cents, file_xml, file_hash, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING `+runColumns,
		run.Reference, run.AccountID, run.ExecutionDate, run.MessageID, run.ItemCount,
		run.SumCents, fileXML, run.FileHash, run.CreatedBy))
	if err != nil {
		if isUniqueViolationOn(err, "payment_run_reference_key") {
			return PaymentRun{}, ErrRunReferenceExists
		}
		return PaymentRun{}, fmt.Errorf("kasse: insert run: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO payment_run_item (run_id, order_id, end_to_end_id, amount_cents)
			 VALUES ($1,$2,$3,$4)`,
			saved.ID, it.OrderID, it.EndToEndID, it.AmountCents); err != nil {
			if isUniqueViolationOn(err, "payment_run_item_order_key") {
				return PaymentRun{}, ErrOrderAlreadyPaid
			}
			return PaymentRun{}, fmt.Errorf("kasse: insert run item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return PaymentRun{}, fmt.Errorf("kasse: commit run: %w", err)
	}
	return saved, nil
}

// isUniqueViolationOn reports whether err is a unique-constraint violation on the
// named constraint.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
}
