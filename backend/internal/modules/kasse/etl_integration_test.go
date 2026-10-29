package kasse_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

// TestCounterBookETLReconcilesAndIsIdempotent drives the legacy counter book
// (kbs.schalterbuch) into the canonical cash book. It proves the sign -> kind
// mapping, the computed closing balance per day, re-run idempotency (bookings
// upsert by legacy reference) and fail-closed on a bad row.
func TestCounterBookETLReconcilesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := kasse.NewService(kasse.NewRepo(pool), audit.NewStore(pool))
	runner := etl.NewRunner(audit.NewStore(pool)).WithLedger(etl.NewAuditLedger(audit.NewStore(pool)))

	d1 := uniqueDate(t)
	d2 := d1.AddDate(0, 0, 1)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE legacy_schalterbuch (
			scha_dat date, betrag numeric(14,2), grund1 varchar(27), grund2 varchar(27),
			scha_nr int, jahr varchar(4)
		)`); err != nil {
		t.Fatalf("create legacy_schalterbuch: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO legacy_schalterbuch VALUES
			($1, 123.45, 'Barzahlung',       '',         1, '2026'),
			($1, -20.00, 'Wechselgeld',      'Rueckgabe',2, '2026'),
			($2,  50.00, '',                 '',         3, '2026')`,
		d1, d2); err != nil {
		t.Fatalf("seed legacy_schalterbuch: %v", err)
	}

	ds := etl.Dataset{
		Name:   "kasse/counter-book/" + uniqueSuffix(t),
		Source: kasse.LegacyCounterBookSource(pool, "SELECT scha_dat, betrag::text, grund1, grund2, scha_nr, jahr FROM legacy_schalterbuch ORDER BY scha_dat, scha_nr"),
		Sink:   kasse.NewCounterBookSink(svc),
	}

	first, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first.Source != 3 || first.Loaded != 3 || first.Previous != "" || first.Drift {
		t.Fatalf("report = %+v", first)
	}

	// Day 1: opening 0, income 123.45 - expense 20.00 -> closing 103.45 (10345).
	// Day 2: opening 0, income 50.00 -> closing 50.00 (5000).
	closing := func(date time.Time) int64 {
		t.Helper()
		var id, c int64
		var status string
		if err := pool.QueryRow(ctx, `SELECT id, status, closing_cents FROM cash_book_day WHERE business_date=$1`, date).Scan(&id, &status, &c); err != nil {
			t.Fatalf("read day %s: %v", date, err)
		}
		if status != "closed" {
			t.Fatalf("day %s status = %s, want closed", date, status)
		}
		return c
	}
	if got := closing(d1); got != 10345 {
		t.Fatalf("day1 closing = %d, want 10345", got)
	}
	if got := closing(d2); got != 5000 {
		t.Fatalf("day2 closing = %d, want 5000", got)
	}

	var income, expense int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE b.kind='income'), count(*) FILTER (WHERE b.kind='expense')
		FROM cash_booking b JOIN cash_book_day d ON d.id = b.day_id
		WHERE d.business_date = $1`, d1).Scan(&income, &expense); err != nil {
		t.Fatalf("kind counts: %v", err)
	}
	if income != 1 || expense != 1 {
		t.Fatalf("day1 kinds = %d income / %d expense, want 1/1 (sign -> kind)", income, expense)
	}
	// Blank legacy text becomes a placeholder, not an import failure.
	var purpose string
	if err := pool.QueryRow(ctx, `SELECT b.purpose FROM cash_booking b JOIN cash_book_day d ON d.id=b.day_id WHERE d.business_date=$1`, d2).Scan(&purpose); err != nil {
		t.Fatalf("purpose: %v", err)
	}
	if purpose != "(ohne Buchungstext)" {
		t.Fatalf("blank purpose = %q", purpose)
	}

	// Idempotent re-run: same checksum, no drift, no duplicate bookings.
	second, err := runner.Run(ctx, ds, "etl-bot")
	if err != nil {
		t.Fatalf("re-Run: %v", err)
	}
	if second.Checksum != first.Checksum || second.Previous != first.Checksum || second.Drift {
		t.Fatalf("unchanged re-run: %+v", second)
	}
	var total int
	pool.QueryRow(ctx, `SELECT count(*) FROM cash_booking WHERE legacy_ref LIKE 'kbs.schalterbuch/%'`).Scan(&total)
	if total != 3 {
		t.Fatalf("booking count after re-run = %d, want 3", total)
	}

	// Fail closed: a zero-amount row has no kind, so the day's import fails and
	// its state is unchanged.
	if _, err := pool.Exec(ctx, `INSERT INTO legacy_schalterbuch VALUES ($1, 0.00, 'null', '', 4, '2026')`, d2); err != nil {
		t.Fatalf("insert bad row: %v", err)
	}
	if _, err := runner.Run(ctx, ds, "etl-bot"); err == nil {
		t.Fatalf("a zero-amount row should fail the import")
	}
	var day2 int
	pool.QueryRow(ctx, `SELECT count(*) FROM cash_booking b JOIN cash_book_day d ON d.id=b.day_id WHERE d.business_date=$1`, d2).Scan(&day2)
	if day2 != 1 || closing(d2) != 5000 {
		t.Fatalf("day2 changed after a failed run: bookings=%d closing=%d", day2, closing(d2))
	}

	if err := audit.NewStore(pool).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
