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

// Reference-dataset namespaces in the canonical catalogue.
const (
	// BankDirectoryNamespace holds the German bank-code directory (legacy kbs.blz).
	BankDirectoryNamespace = "bank_leit_zahl"
	// InsurersNamespace holds the statutory health insurers (legacy
	// sva4.k_krankenkasse), reference data for the Personal module.
	InsurersNamespace = "krankenkasse"
)

// These are the measured legacy queries for the two reference datasets whose
// source tables hold real rows in the productive source (unlike most module
// tables). The tables are addressed unqualified so the dataset pool's search_path
// selects the schema (kbs for blz, sva4 for k_krankenkasse); fixed-width char
// columns are trimmed and cast to text.
const (
	bankDirectoryQuery = `SELECT btrim(blz_key::text), btrim(bname::text), btrim(langbezeichnung::text), 0, true
	                        FROM blz WHERE blz_key IS NOT NULL`
	insurersQuery = `SELECT btrim(key_krankenkasse::text), btrim(kurz_krankenkasse::text), btrim(text_krankenkasse::text), coalesce(sortorder, 0), true
	                   FROM k_krankenkasse WHERE key_krankenkasse IS NOT NULL`
)

// BankDirectoryDataset builds the ETL dataset for the German bank-code directory
// (legacy kbs.blz: 3,732 rows) into the catalogue. It is genuinely useful for
// SEPA/IBAN work — BLZ -> name/city/BIC.
func BankDirectoryDataset(pool *pgxpool.Pool, svc *ValueService) etl.Dataset {
	return etl.Dataset{
		Name:   "lookup/" + BankDirectoryNamespace,
		Source: LegacyCatalogueSource(pool, BankDirectoryNamespace, bankDirectoryQuery),
		Sink:   NewCatalogueSink(svc),
	}
}

// InsurersDataset builds the ETL dataset for the statutory health insurers
// (legacy sva4.k_krankenkasse: 1,677 rows) into the catalogue.
func InsurersDataset(pool *pgxpool.Pool, svc *ValueService) etl.Dataset {
	return etl.Dataset{
		Name:   "lookup/" + InsurersNamespace,
		Source: LegacyCatalogueSource(pool, InsurersNamespace, insurersQuery),
		Sink:   NewCatalogueSink(svc),
	}
}

// StandardDatasets returns the reference datasets sourced from the productive
// legacy data (as opposed to the coded value sets seeded in code).
func StandardDatasets(pool *pgxpool.Pool, svc *ValueService) []etl.Dataset {
	return []etl.Dataset{
		BankDirectoryDataset(pool, svc),
		InsurersDataset(pool, svc),
	}
}
