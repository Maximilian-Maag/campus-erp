package etl

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuerySource extracts a dataset from a legacy database with a SQL query. It is
// schema-driven: the dataset supplies the query and the row→Record mapper, so a
// new legacy table is a new query, not new plumbing.
type QuerySource struct {
	Pool *pgxpool.Pool
	SQL  string
	Scan func(rows pgx.Rows) (Record, error)
}

// Extract implements Source.
func (q QuerySource) Extract(ctx context.Context) ([]Record, error) {
	rows, err := q.Pool.Query(ctx, q.SQL)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := q.Scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
