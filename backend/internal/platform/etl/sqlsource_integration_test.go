package etl_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/dbtest"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func scanPair(rows pgx.Rows) (etl.Record, error) {
	var code, label string
	if err := rows.Scan(&code, &label); err != nil {
		return etl.Record{}, err
	}
	return etl.Record{Key: code, Fields: map[string]string{"label": label}}, nil
}

func TestQuerySourceExtracts(t *testing.T) {
	pool := dbtest.Pool(t)
	src := etl.QuerySource{
		Pool: pool,
		SQL:  "SELECT g::text, 'v' || g FROM generate_series(1, 3) g ORDER BY g",
		Scan: scanPair,
	}
	recs, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(recs) != 3 || recs[0].Key != "1" || recs[0].Fields["label"] != "v1" {
		t.Fatalf("records = %+v", recs)
	}
}

func TestQuerySourceReportsQueryError(t *testing.T) {
	pool := dbtest.Pool(t)
	src := etl.QuerySource{Pool: pool, SQL: "SELECT 1 FROM no_such_table_here", Scan: scanPair}
	if _, err := src.Extract(context.Background()); err == nil {
		t.Fatalf("expected a query error")
	}
}

func TestQuerySourceReportsScanError(t *testing.T) {
	pool := dbtest.Pool(t)
	// The second column is text that cannot scan into an int, so the scan fails.
	src := etl.QuerySource{Pool: pool, SQL: "SELECT 'a', 'not-a-number'", Scan: func(rows pgx.Rows) (etl.Record, error) {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return etl.Record{}, err
		}
		return etl.Record{Key: code}, nil
	}}
	if _, err := src.Extract(context.Background()); err == nil {
		t.Fatalf("expected a scan error")
	}
}
