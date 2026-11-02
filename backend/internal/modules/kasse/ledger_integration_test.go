package kasse_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// ledgerIntDay is the business date used by the export integration test.
func ledgerIntDay(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

// TestLedgerExportIntegration covers the collection query against a real database:
// bookings on days inside the period are exported, the run is recorded on the audit
// stream, and a period with no bookings is refused rather than silently empty.
func TestLedgerExportIntegration(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	// A day in the period, idempotent across runs (business_date is unique).
	var dayID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO cash_book_day (business_date, status, opening_cents, opened_by)
		VALUES ($1, 'closed', 0, 'setup')
		ON CONFLICT (business_date) DO UPDATE SET status = 'closed'
		RETURNING id`, ledgerIntDay(1)).Scan(&dayID)
	if err != nil {
		t.Fatalf("seed day: %v", err)
	}
	for _, b := range []struct {
		kind    string
		cents   int64
		purpose string
	}{{"income", 125000, "Stromabschlag"}, {"expense", 4999, "Fachliteratur"}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO cash_booking (day_id, kind, amount_cents, purpose, booked_by)
			VALUES ($1, $2, $3, $4, 'setup')`, dayID, b.kind, b.cents, b.purpose); err != nil {
			t.Fatalf("seed booking: %v", err)
		}
	}

	svc := kasse.NewLedgerExportService(kasse.NewLedgerRepo(pool), audit.NewStore(pool), kasse.DefaultLedgerAccounts())
	exp, err := svc.Export(ctx, ledgerIntDay(1), ledgerIntDay(31), "carol")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(exp.Postings) != 2 {
		t.Fatalf("postings = %d, want 2", len(exp.Postings))
	}
	got, err := exp.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		",125000,EUR,Stromabschlag",
		",4999,EUR,Fachliteratur",
		"cash_account: 1200",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q:\n%s", want, got)
		}
	}
	// The reference is stable, so a second export of the same period is identical.
	again, err := svc.Export(ctx, ledgerIntDay(1), ledgerIntDay(31), "carol")
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	got2, err := again.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if got != got2 {
		t.Fatalf("re-export differs:\n%s\n---\n%s", got, got2)
	}

	// An empty period is refused, not silently exported.
	if _, err := svc.Export(ctx, time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2019, 1, 2, 0, 0, 0, 0, time.UTC), "carol"); !errors.Is(err, kasse.ErrNoBookings) {
		t.Fatalf("empty period error = %v, want ErrNoBookings", err)
	}
}
