package kasse_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// TestVATETLReconcilesAndIsIdempotent drives the legacy VAT catalogue
// (kbs.mwst_art) through the ETL into the canonical vat_type: it proves the
// percent -> basis-points and flag -> kind mapping, reconciliation, re-run
// idempotency, in-place update, and fail-closed on a bad row.
func TestVATETLReconcilesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewTaxService(kasse.NewTaxRepo(pool), audit.NewStore(pool))

	s := uniqueSuffix(t)
	code := func(x string) string { return "V" + s + x } // <= 8 chars, unique per run

	if _, err := pool.Exec(ctx, `
		CREATE TABLE legacy_mwst_art (
			mw_art       varchar(8),
			mw_bez       varchar(50),
			mw_kz        numeric,
			mw_typ       varchar(1),
			mw_vorzeichen varchar(1)
		)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO legacy_mwst_art (mw_art, mw_bez, mw_kz, mw_typ, mw_vorzeichen) VALUES
			($1, 'Regelsteuersatz',          '19.00', 'U', '+'),
			($2, 'ermaessigt',               '7',     '',  ''),
			($3, 'steuerfrei',               '0.00',  'V', '-'),
			($4, 'Vorsteuer ig. Erwerb',     '19.00', 'V', '-'),
			($5, 'Ausfuhr',                  '0.00',  'U', '+')`,
		code("A"), code("B"), code("C"), code("D"), code("E")); err != nil {
		t.Fatalf("seed legacy table: %v", err)
	}

	ds := etl.Dataset{
		Name:   "kasse/vat/" + s,
		Source: kasse.LegacyVATSource(pool, "SELECT mw_art, mw_bez, mw_kz::text, mw_typ FROM legacy_mwst_art ORDER BY mw_art"),
		Sink:   kasse.NewVATSink(svc),
	}
	runner := etl.NewRunner(audit.NewStore(pool)).WithLedger(etl.NewAuditLedger(audit.NewStore(pool)))

	first, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first.Source != 5 || first.Loaded != 5 || first.Checksum == "" {
		t.Fatalf("report = %+v", first)
	}
	if first.Previous != "" || first.Drift {
		t.Fatalf("a first run has no history: %+v", first)
	}

	want := []struct {
		code string
		rate int64
		kind kasse.VATKind
	}{
		{code("A"), 1900, kasse.VATOutput},
		{code("B"), 700, kasse.VATOutput}, // empty flag -> output (documented inference)
		{code("C"), 0, kasse.VATInput},
		{code("D"), 1900, kasse.VATInput},
		{code("E"), 0, kasse.VATOutput},
	}
	for _, w := range want {
		got, err := svc.VATType(ctx, w.code)
		if err != nil {
			t.Fatalf("VATType(%s): %v", w.code, err)
		}
		if got.RateBasisPoints != w.rate || got.Kind != w.kind {
			t.Fatalf("%s = rate %d/%s, want %d/%s", w.code, got.RateBasisPoints, got.Kind, w.rate, w.kind)
		}
	}

	// Re-run: same checksum, no drift, not duplicated (upsert).
	second, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("re-Run: %v", err)
	}
	if second.Checksum != first.Checksum || second.Previous != first.Checksum || second.Drift {
		t.Fatalf("unchanged re-run: checksum=%s prev=%s drift=%t", second.Checksum, second.Previous, second.Drift)
	}

	// Change a rate: drift flagged, value updated in place.
	if _, err := pool.Exec(ctx, `UPDATE legacy_mwst_art SET mw_kz = '7.00' WHERE mw_art = $1`, code("A")); err != nil {
		t.Fatalf("update legacy: %v", err)
	}
	third, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("Run after change: %v", err)
	}
	if third.Checksum == first.Checksum || !third.Drift {
		t.Fatalf("changed source must drift: %+v", third)
	}
	if got, _ := svc.VATType(ctx, code("A")); got.RateBasisPoints != 700 {
		t.Fatalf("rate not updated in place: %+v", got)
	}

	// A row that cannot be canonicalised fails the run with no partial load. The
	// row is valid to the legacy schema (a numeric rate) but the canonical model
	// rejects a rate above 100 %.
	if _, err := pool.Exec(ctx, `INSERT INTO legacy_mwst_art (mw_art, mw_bez, mw_kz, mw_typ) VALUES ($1, 'bad rate', '200', 'U')`, code("Z")); err != nil {
		t.Fatalf("insert bad row: %v", err)
	}
	if _, err := runner.Run(ctx, ds, "etl-bot"); err == nil {
		t.Fatalf("bad row should fail the run")
	}
	if _, err := svc.VATType(ctx, code("Z")); !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("bad row leaked into the catalogue: %v", err)
	}
	if got, _ := svc.VATType(ctx, code("A")); got.RateBasisPoints != 700 {
		t.Fatalf("good values disturbed by a failed run: %+v", got)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
