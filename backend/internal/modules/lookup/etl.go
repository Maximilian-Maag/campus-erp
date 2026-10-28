package lookup

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// CatalogueSink loads extracted legacy key-table rows into the canonical
// catalogue via the value service. It validates the whole batch before writing,
// so a bad row fails the run without leaving a partial load behind.
type CatalogueSink struct {
	svc *ValueService
}

// NewCatalogueSink wires the sink to the lookup service.
func NewCatalogueSink(svc *ValueService) CatalogueSink { return CatalogueSink{svc: svc} }

// Load implements etl.Sink.
func (c CatalogueSink) Load(ctx context.Context, records []etl.Record, by string) (int, error) {
	values := make([]Value, 0, len(records))
	for _, r := range records {
		order, err := strconv.Atoi(orZero(r.Fields["sort_order"]))
		if err != nil {
			return 0, fmt.Errorf("sort_order %q: %w", r.Fields["sort_order"], err)
		}
		v, err := NewValue(r.Fields["namespace"], r.Fields["code"], r.Fields["short_text"], r.Fields["long_text"], order, r.Fields["active"] == "true")
		if err != nil {
			return 0, err
		}
		values = append(values, v)
	}
	saved, err := c.svc.UpsertMany(ctx, values, by)
	if err != nil {
		return 0, err
	}
	return len(saved), nil
}

// LegacyCatalogueSource reads one legacy key table into catalogue records. The
// legacy schema keeps one table per value set with (code, short text, long text,
// sort order, active); the query supplies the columns and the namespace names the
// resulting catalogue.
func LegacyCatalogueSource(pool *pgxpool.Pool, namespace, query string) etl.QuerySource {
	return etl.QuerySource{
		Pool: pool,
		SQL:  query,
		Scan: func(rows pgx.Rows) (etl.Record, error) {
			var code, short, long string
			var order int
			var active bool
			if err := rows.Scan(&code, &short, &long, &order, &active); err != nil {
				return etl.Record{}, err
			}
			return etl.Record{
				Key: namespace + "/" + code,
				Fields: map[string]string{
					"namespace":  namespace,
					"code":       code,
					"short_text": short,
					"long_text":  long,
					"sort_order": strconv.Itoa(order),
					"active":     strconv.FormatBool(active),
				},
			}, nil
		},
	}
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}
