package einvoice_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — integration tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return pool
}

func uniqueSuffix(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405.000000000")
}

func newService(t *testing.T) *einvoice.InvoiceService {
	t.Helper()
	pool := testPool(t)
	return einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), audit.NewStore(pool))
}

func sampleInvoice(t *testing.T) core.Invoice {
	t.Helper()
	return core.Invoice{
		Number:         "RE-" + uniqueSuffix(t),
		IssueDate:      time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		TypeCode:       "380",
		Currency:       "EUR",
		BuyerReference: "04011000-12345-03",
		Seller: core.Party{
			Name: "Universität Musterstadt", Street: "Musterstraße 1", City: "Musterstadt",
			PostalCode: "12345", Country: "DE", VatID: "DE123456789",
			EndpointID: "rechnung@uni-musterstadt.de", EndpointScheme: "EM", Email: "rechnung@uni-musterstadt.de",
		},
		Buyer: core.Party{
			Name: "Stadt Musterstadt", Street: "Rathausplatz 1", City: "Musterstadt",
			PostalCode: "12345", Country: "DE",
		},
		PaymentMeansCode: "58",
		PayeeIBAN:        "DE75512108001245126199",
		PaymentTerms:     "Zahlbar innerhalb von 30 Tagen ohne Abzug.",
		Lines: []core.Line{
			{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000, UnitCode: "C62",
				UnitPriceCents: 28879, NetCents: 28879, VatCategory: "S", VatRateBP: 700},
			{LineNo: 2, Description: "Literatur", QuantityMilli: 2000, UnitCode: "C62",
				UnitPriceCents: 1304, NetCents: 2608, VatCategory: "S", VatRateBP: 1900},
		},
	}
}

func TestInvoiceCreateGetRender(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	saved, err := svc.Create(ctx, sampleInvoice(t), "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("ID not set")
	}

	got, err := svc.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Number != saved.Number || got.Seller.Name != saved.Seller.Name || got.Buyer.Name != saved.Buyer.Name {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.Lines) != 2 || got.Lines[1].NetCents != 2608 || got.Lines[1].VatRateBP != 1900 {
		t.Fatalf("lines mismatch: %+v", got.Lines)
	}

	doc, err := svc.Render(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(doc, "<cbc:ID>"+saved.Number+"</cbc:ID>") {
		t.Fatalf("document missing invoice id:\n%s", doc)
	}
	if !strings.Contains(doc, core.CustomizationID) {
		t.Fatal("document missing the XRechnung customization id")
	}
	if !strings.Contains(doc, "<cbc:PayableAmount currencyID=\"EUR\">340.05</cbc:PayableAmount>") {
		t.Fatalf("document has wrong payable amount:\n%s", doc)
	}
}

func TestInvoiceValidationAndDuplicates(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.Create(ctx, sampleInvoice(t), "  "); !errors.Is(err, einvoice.ErrActorRequired) {
		t.Fatalf("blank actor: %v", err)
	}
	bad := sampleInvoice(t)
	bad.Number = " "
	if _, err := svc.Create(ctx, bad, "alice"); !errors.Is(err, core.ErrNumberRequired) {
		t.Fatalf("no number: %v", err)
	}

	inv := sampleInvoice(t)
	if _, err := svc.Create(ctx, inv, "alice"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.Create(ctx, inv, "alice"); !errors.Is(err, einvoice.ErrNumberExists) {
		t.Fatalf("duplicate: %v", err)
	}

	if _, err := svc.Get(ctx, 9223372036854775807); !errors.Is(err, einvoice.ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
	if _, err := svc.Render(ctx, 9223372036854775807); !errors.Is(err, einvoice.ErrNotFound) {
		t.Fatalf("render not found: %v", err)
	}
}

func TestInvoiceAuditChain(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := audit.NewStore(pool)
	svc := einvoice.NewInvoiceService(einvoice.NewInvoiceRepo(pool), store)

	if _, err := svc.Create(ctx, sampleInvoice(t), "alice"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}
