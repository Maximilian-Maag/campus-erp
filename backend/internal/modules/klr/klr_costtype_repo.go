package klr

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// isForeignKeyViolationOn reports whether err is a foreign-key violation on the named constraint.
func isForeignKeyViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation && pgErr.ConstraintName == constraint
}

const costTypeColumns = `id, code, name, created_by`

func scanCostType(row pgx.Row) (CostType, error) {
	var t CostType
	err := row.Scan(&t.ID, &t.Code, &t.Name, &t.CreatedBy)
	return t, err
}

// CreateCostType stores a cost type.
func (r *Repo) CreateCostType(ctx context.Context, t CostType) (CostType, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO cost_type (code, name, created_by) VALUES ($1,$2,$3) RETURNING `+costTypeColumns,
		t.Code, t.Name, t.CreatedBy)
	out, err := scanCostType(row)
	if err != nil {
		if isUniqueViolationOn(err, "cost_type_code_key") {
			return CostType{}, ErrCostTypeExists
		}
		return CostType{}, fmt.Errorf("klr: create cost type: %w", err)
	}
	return out, nil
}

// GetCostType loads a cost type by id.
func (r *Repo) GetCostType(ctx context.Context, id int64) (CostType, error) {
	out, err := scanCostType(r.pool.QueryRow(ctx,
		`SELECT `+costTypeColumns+` FROM cost_type WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return CostType{}, fmt.Errorf("klr: cost type %d: %w", id, ErrCostTypeNotFound)
	}
	if err != nil {
		return CostType{}, fmt.Errorf("klr: get cost type: %w", err)
	}
	return out, nil
}

// CostTypes lists every cost type by code, which is the order an operator reads them in.
func (r *Repo) CostTypes(ctx context.Context) ([]CostType, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+costTypeColumns+` FROM cost_type ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("klr: list cost types: %w", err)
	}
	defer rows.Close()
	out := []CostType{}
	for rows.Next() {
		t, err := scanCostType(rows)
		if err != nil {
			return nil, fmt.Errorf("klr: scan cost type: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list cost types: %w", err)
	}
	return out, nil
}

// MapAccount links an account code to a cost type. An account belongs to exactly one cost type:
// mapping the same code to the same cost type again is idempotent (it returns the existing
// link), mapping it to a different one is refused.
func (r *Repo) MapAccount(ctx context.Context, link CostTypeAccount) (CostTypeAccount, error) {
	var out CostTypeAccount
	err := r.pool.QueryRow(ctx,
		`INSERT INTO cost_type_account (cost_type_id, account_code) VALUES ($1,$2)
		 ON CONFLICT (account_code) DO NOTHING
		 RETURNING id, cost_type_id, account_code`,
		link.CostTypeID, link.AccountCode).Scan(&out.ID, &out.CostTypeID, &out.AccountCode)
	if err == nil {
		return out, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		if isForeignKeyViolationOn(err, "cost_type_account_cost_type_id_fkey") {
			return CostTypeAccount{}, fmt.Errorf("klr: cost type %d: %w", link.CostTypeID, ErrCostTypeNotFound)
		}
		return CostTypeAccount{}, fmt.Errorf("klr: map account: %w", err)
	}
	existing, err := r.accountLink(ctx, link.AccountCode)
	if err != nil {
		return CostTypeAccount{}, err
	}
	if existing.CostTypeID != link.CostTypeID {
		return CostTypeAccount{}, fmt.Errorf("klr: account %q: %w", link.AccountCode, ErrAccountAlreadyMapped)
	}
	return existing, nil
}

func (r *Repo) accountLink(ctx context.Context, accountCode string) (CostTypeAccount, error) {
	var out CostTypeAccount
	err := r.pool.QueryRow(ctx,
		`SELECT id, cost_type_id, account_code FROM cost_type_account WHERE account_code = $1`, accountCode).
		Scan(&out.ID, &out.CostTypeID, &out.AccountCode)
	if err != nil {
		return CostTypeAccount{}, fmt.Errorf("klr: account link: %w", err)
	}
	return out, nil
}

// AccountsOf lists the accounts a cost type owns, by code.
func (r *Repo) AccountsOf(ctx context.Context, costTypeID int64) ([]CostTypeAccount, error) {
	if _, err := r.GetCostType(ctx, costTypeID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, cost_type_id, account_code FROM cost_type_account
		  WHERE cost_type_id = $1 ORDER BY account_code`, costTypeID)
	if err != nil {
		return nil, fmt.Errorf("klr: list accounts: %w", err)
	}
	defer rows.Close()
	out := []CostTypeAccount{}
	for rows.Next() {
		var a CostTypeAccount
		if err := rows.Scan(&a.ID, &a.CostTypeID, &a.AccountCode); err != nil {
			return nil, fmt.Errorf("klr: scan account link: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list accounts: %w", err)
	}
	return out, nil
}

// CostTypeByAccount is the bridge's purpose: which cost type an account belongs to.
func (r *Repo) CostTypeByAccount(ctx context.Context, accountCode string) (CostType, error) {
	out, err := scanCostType(r.pool.QueryRow(ctx,
		`SELECT t.id, t.code, t.name, t.created_by
		   FROM cost_type t JOIN cost_type_account a ON a.cost_type_id = t.id
		  WHERE a.account_code = $1`, accountCode))
	if errors.Is(err, pgx.ErrNoRows) {
		return CostType{}, fmt.Errorf("klr: account %q: %w", accountCode, ErrAccountNotMapped)
	}
	if err != nil {
		return CostType{}, fmt.Errorf("klr: cost type by account: %w", err)
	}
	return out, nil
}

// isUniqueViolationOn reports whether err is a unique-constraint violation on the named
// constraint.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
}
