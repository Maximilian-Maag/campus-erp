package lookup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// TestCatalogueETLReconcilesAndIsIdempotent exercises the first real pipeline: a
// legacy key table (synthetic, in this test's schema) into the canonical
// catalogue. It proves the run reconciles, re-runs are idempotent, a bad row
// fails closed without a partial load, and every run is audited.
func TestCatalogueETLReconcilesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := lookup.NewValueService(lookup.NewValueRepo(pool), audit.NewStore(pool))
	ns := "legacy_cat_" + suffix(t)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE legacy_key (
			code       text primary key,
			short_text text,
			long_text  text,
			sort_order int,
			active     boolean
		)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO legacy_key (code, short_text, long_text, sort_order, active) VALUES
			('S', 'Regelsteuersatz', 'Standard rate', 1, true),
			('Z', 'ermaessigt',     'Reduced rate',  2, true),
			('X', 'alt',            'Retired',       3, false)`); err != nil {
		t.Fatalf("seed legacy table: %v", err)
	}

	ds := etl.Dataset{
		Name:   ns,
		Source: lookup.LegacyCatalogueSource(pool, ns, "SELECT code, short_text, long_text, sort_order, active FROM legacy_key ORDER BY code"),
		Sink:   lookup.NewCatalogueSink(svc),
	}
	runner := etl.NewRunner(audit.NewStore(pool)).WithLedger(etl.NewAuditLedger(audit.NewStore(pool)))

	first, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first.Source != 3 || first.Loaded != 3 || first.Checksum == "" {
		t.Fatalf("report = %+v", first)
	}
	if first.Previous != "" || first.Drift {
		t.Fatalf("a first run has no history: %+v", first)
	}

	values, err := svc.List(ctx, ns)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("canonical values = %d, want 3", len(values))
	}
	if values[0].Code != "S" || values[0].ShortText != "Regelsteuersatz" || !values[0].Active {
		t.Fatalf("first value = %+v", values[0])
	}
	if values[2].Active {
		t.Fatalf("retired value should be inactive: %+v", values[2])
	}

	// Re-run: same checksum, still three rows (upsert, not append).
	second, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("re-Run: %v", err)
	}
	if second.Checksum != first.Checksum {
		t.Fatalf("checksum drifted on re-run: %s vs %s", first.Checksum, second.Checksum)
	}
	if second.Previous != first.Checksum || second.Drift {
		t.Fatalf("an unchanged re-run must not drift: %+v", second)
	}
	if again, _ := svc.List(ctx, ns); len(again) != 3 {
		t.Fatalf("re-run duplicated rows: %d", len(again))
	}

	// Change a source row: the checksum must move and the value must update.
	if _, err := pool.Exec(ctx, `UPDATE legacy_key SET short_text = 'Standard' WHERE code = 'S'`); err != nil {
		t.Fatalf("update legacy: %v", err)
	}
	third, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("Run after change: %v", err)
	}
	if third.Checksum == first.Checksum {
		t.Fatalf("checksum did not move after a source change")
	}
	if third.Previous != first.Checksum || !third.Drift {
		t.Fatalf("a changed source must be flagged as drift: %+v", third)
	}
	if v, _ := svc.Get(ctx, ns, "S"); v.ShortText != "Standard" {
		t.Fatalf("value not updated in place: %+v", v)
	}

	// A row that cannot be canonicalised must fail the run with no partial write.
	if _, err := pool.Exec(ctx, `INSERT INTO legacy_key (code, short_text, sort_order, active) VALUES ('', 'blank code', 4, true)`); err != nil {
		t.Fatalf("insert bad row: %v", err)
	}
	if _, err := runner.Run(ctx, ds, "etl-bot"); err == nil {
		t.Fatalf("bad row should fail the run")
	} else if errors.Is(err, etl.ErrReconcile) {
		t.Fatalf("a validation failure is a load error, not reconciliation loss: %v", err)
	}
	if after, _ := svc.List(ctx, ns); len(after) != 3 {
		t.Fatalf("partial load leaked: %d rows, want 3", len(after))
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestReferenceDatasetsImport drives the two reference datasets whose source
// tables hold real rows in the productive source — the bank-code directory
// (kbs.blz) and the health insurers (sva4.k_krankenkasse) — against synthetic
// tables of the same shape, and checks re-run idempotency.
func TestReferenceDatasetsImport(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := lookup.NewValueService(lookup.NewValueRepo(pool), audit.NewStore(pool))
	runner := etl.NewRunner(audit.NewStore(pool)).WithLedger(etl.NewAuditLedger(audit.NewStore(pool)))

	if _, err := pool.Exec(ctx, `CREATE TABLE blz (blz_key char(8), bname char(27), bort char(35), bic char(11), langbezeichnung char(58))`); err != nil {
		t.Fatalf("create blz: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO blz VALUES
		('10000000', 'BBk Berlin',   'Berlin', 'MARKDEF1100', 'Bundesbank'),
		('10010010', 'Postbank Berlin', 'Berlin', 'PBNKDEFFXXX', 'Postbank')`); err != nil {
		t.Fatalf("seed blz: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE k_krankenkasse (key_krankenkasse varchar, kurz_krankenkasse varchar, text_krankenkasse varchar, sortorder int)`); err != nil {
		t.Fatalf("create insurers: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO k_krankenkasse VALUES
		('48676321', 'BKK DWL AG', 'BKK DWL AG', 1),
		('1000774',  'BKK',        'Bahn-BKK',   2)`); err != nil {
		t.Fatalf("seed insurers: %v", err)
	}

	datasets := lookup.StandardDatasets(pool, svc)
	if len(datasets) != 2 {
		t.Fatalf("StandardDatasets = %d, want 2", len(datasets))
	}
	for _, ds := range datasets {
		rep, err := runner.Run(ctx, ds, "etl-bot")
		if err != nil {
			t.Fatalf("Run %s: %v", ds.Name, err)
		}
		if rep.Source != 2 || rep.Loaded != 2 {
			t.Fatalf("report %s = %+v", ds.Name, rep)
		}
	}

	bank, err := svc.List(ctx, lookup.BankDirectoryNamespace)
	if err != nil || len(bank) != 2 {
		t.Fatalf("bank directory = %d, %v", len(bank), err)
	}
	entry, err := svc.Get(ctx, lookup.BankDirectoryNamespace, "10000000")
	if err != nil || entry.ShortText != "BBk Berlin" || entry.LongText != "Bundesbank" {
		t.Fatalf("bank entry = %+v, %v", entry, err)
	}
	if insurers, err := svc.List(ctx, lookup.InsurersNamespace); err != nil || len(insurers) != 2 {
		t.Fatalf("insurers = %d, %v", len(insurers), err)
	}

	// Idempotent re-run: same rows, no duplicates.
	second, err := runner.Run(ctx, datasets[0], "etl-bot")
	if err != nil || second.Drift {
		t.Fatalf("re-run: %v %+v", err, second)
	}
	if bank, _ := svc.List(ctx, lookup.BankDirectoryNamespace); len(bank) != 2 {
		t.Fatalf("re-run duplicated bank entries: %d", len(bank))
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
