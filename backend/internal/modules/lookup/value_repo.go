package lookup

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValueRepository is the persistence port for catalogue values.
type ValueRepository interface {
	Upsert(ctx context.Context, v Value) (Value, error)
	Get(ctx context.Context, namespace, code string) (Value, error)
	List(ctx context.Context, namespace string) ([]Value, error)
}

// ValueRepo is the PostgreSQL implementation.
type ValueRepo struct{ pool *pgxpool.Pool }

// NewValueRepo returns a ValueRepo.
func NewValueRepo(pool *pgxpool.Pool) *ValueRepo { return &ValueRepo{pool: pool} }

const valueColumns = `namespace, code, short_text, long_text, sort_order, active`

func scanValue(row pgx.Row) (Value, error) {
	var v Value
	err := row.Scan(&v.Namespace, &v.Code, &v.ShortText, &v.LongText, &v.SortOrder, &v.Active)
	return v, err
}

// Upsert inserts or updates a value in place (idempotent seeding).
func (r *ValueRepo) Upsert(ctx context.Context, v Value) (Value, error) {
	saved, err := scanValue(r.pool.QueryRow(ctx,
		`INSERT INTO lookup_value (namespace, code, short_text, long_text, sort_order, active)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (namespace, code) DO UPDATE
		   SET short_text = EXCLUDED.short_text,
		       long_text  = EXCLUDED.long_text,
		       sort_order = EXCLUDED.sort_order,
		       active     = EXCLUDED.active
		 RETURNING `+valueColumns,
		v.Namespace, v.Code, v.ShortText, v.LongText, v.SortOrder, v.Active))
	if err != nil {
		return Value{}, fmt.Errorf("lookup: upsert value: %w", err)
	}
	return saved, nil
}

// Get loads one value.
func (r *ValueRepo) Get(ctx context.Context, namespace, code string) (Value, error) {
	v, err := scanValue(r.pool.QueryRow(ctx,
		`SELECT `+valueColumns+` FROM lookup_value WHERE namespace = $1 AND code = $2`, namespace, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return Value{}, fmt.Errorf("lookup: %s/%s: %w", namespace, code, ErrNotFound)
	}
	if err != nil {
		return Value{}, fmt.Errorf("lookup: get value: %w", err)
	}
	return v, nil
}

// List returns a namespace's values in sort order.
func (r *ValueRepo) List(ctx context.Context, namespace string) ([]Value, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+valueColumns+` FROM lookup_value WHERE namespace = $1 ORDER BY sort_order, code`, namespace)
	if err != nil {
		return nil, fmt.Errorf("lookup: list values: %w", err)
	}
	defer rows.Close()
	var out []Value
	for rows.Next() {
		v, err := scanValue(rows)
		if err != nil {
			return nil, fmt.Errorf("lookup: scan value: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
