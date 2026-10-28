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

// TestTaxOfficeAndFoundationETL drives kbs.finanzamt and kbs.stift through the
// ETL: two more dataset shapes (an int year and a carry-over flag), reconciled and
// idempotent, fail-closed on a bad row.
func TestTaxOfficeAndFoundationETL(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewTaxService(kasse.NewTaxRepo(pool), audit.NewStore(pool))
	runner := etl.NewRunner(audit.NewStore(pool)).WithLedger(etl.NewAuditLedger(audit.NewStore(pool)))

	const year = 2999 // outside any suite fixture; uniqueness comes from the number
	officeNo := "F" + uniqueSuffix(t)
	foundNo := "S" + uniqueSuffix(t)
	iban := uniqueIBAN(t)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE legacy_finanzamt (
			jahr int, finanzamt_nr varchar(20), bezeichnung varchar(60), str varchar(60),
			plz varchar(10), ort varchar(60), inhaber varchar(60), iban varchar(35), bic varchar(11)
		)`); err != nil {
		t.Fatalf("create legacy_finanzamt: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO legacy_finanzamt VALUES ($1,$2,'Finanzamt Test','Hauptstr 1','88046','Friedrichshafen','Land Baden-Wuerttemberg',$3,'SOLADES1KNZ')`,
		year, officeNo, iban); err != nil {
		t.Fatalf("seed legacy_finanzamt: %v", err)
	}

	if _, err := pool.Exec(ctx, `CREATE TABLE legacy_stift (jahr int, stift_nr varchar(20), bez varchar(30), kz_uebern char(1))`); err != nil {
		t.Fatalf("create legacy_stift: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO legacy_stift VALUES ($1,$2,'Teststiftung','J'), ($1,$3,'Sammelstiftung','')`,
		year, foundNo, foundNo+"b"); err != nil {
		t.Fatalf("seed legacy_stift: %v", err)
	}

	officeDS := etl.Dataset{
		Name:   "kasse/tax-office/" + officeNo,
		Source: kasse.LegacyTaxOfficeSource(pool, "SELECT jahr, finanzamt_nr, bezeichnung, str, plz, ort, inhaber, iban, bic FROM legacy_finanzamt"),
		Sink:   kasse.NewTaxOfficeSink(svc),
	}
	foundDS := etl.Dataset{
		Name:   "kasse/foundation/" + foundNo,
		Source: kasse.LegacyFoundationSource(pool, "SELECT jahr, stift_nr, bez, kz_uebern FROM legacy_stift"),
		Sink:   kasse.NewFoundationSink(svc),
	}

	for _, ds := range []etl.Dataset{officeDS, foundDS} {
		rep, err := runner.Run(ctx, ds, "etl-bot")
		if err != nil {
			t.Fatalf("Run %s: %v", ds.Name, err)
		}
		if rep.Source == 0 || rep.Loaded != rep.Source || rep.Previous != "" || rep.Drift {
			t.Fatalf("report %s = %+v", ds.Name, rep)
		}
	}

	// Canonical state.
	var gotIBAN, gotBIC string
	if err := pool.QueryRow(ctx, `SELECT iban, bic FROM tax_office WHERE year=$1 AND number=$2`, year, officeNo).Scan(&gotIBAN, &gotBIC); err != nil {
		t.Fatalf("read tax_office: %v", err)
	}
	if gotIBAN != iban || gotBIC != "SOLADES1KNZ" {
		t.Fatalf("tax_office = %s/%s", gotIBAN, gotBIC)
	}
	var carry bool
	if err := pool.QueryRow(ctx, `SELECT carry_over FROM foundation WHERE year=$1 AND number=$2`, year, foundNo).Scan(&carry); err != nil {
		t.Fatalf("read foundation: %v", err)
	}
	if !carry {
		t.Fatalf("foundation carry_over = false, want true for flag J")
	}
	if err := pool.QueryRow(ctx, `SELECT carry_over FROM foundation WHERE year=$1 AND number=$2`, year, foundNo+"b").Scan(&carry); err != nil {
		t.Fatalf("read foundation 2: %v", err)
	}
	if carry {
		t.Fatalf("foundation carry_over = true, want false for empty flag")
	}

	// Idempotent re-run.
	second, err := runner.Run(ctx, officeDS, "etl-bot")
	if err != nil || second.Drift {
		t.Fatalf("unchanged re-run should not drift: %v %+v", err, second)
	}
	var offices, foundations int
	pool.QueryRow(ctx, `SELECT count(*) FROM tax_office WHERE year=$1`, year).Scan(&offices)
	pool.QueryRow(ctx, `SELECT count(*) FROM foundation WHERE year=$1`, year).Scan(&foundations)
	if offices != 1 || foundations != 2 {
		t.Fatalf("counts after re-run: offices=%d foundations=%d", offices, foundations)
	}

	// Fail closed: a foundation with no description is rejected, no partial write.
	if _, err := pool.Exec(ctx, `INSERT INTO legacy_stift VALUES ($1, $2, '', '')`, year, foundNo+"c"); err != nil {
		t.Fatalf("insert bad foundation: %v", err)
	}
	if _, err := runner.Run(ctx, foundDS, "etl-bot"); err == nil {
		t.Fatalf("blank description should fail the import")
	}
	var bad int
	pool.QueryRow(ctx, `SELECT count(*) FROM foundation WHERE number=$1`, foundNo+"c").Scan(&bad)
	if bad != 0 {
		t.Fatalf("bad foundation leaked: %d", bad)
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
