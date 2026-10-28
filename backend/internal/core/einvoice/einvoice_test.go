package einvoice

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

func sample() Invoice {
	return Invoice{
		Number:         "RE-2026-0001",
		IssueDate:      time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
		TypeCode:       "380",
		Currency:       "EUR",
		BuyerReference: "04011000-12345-03",
		Seller: Party{
			Name:           "Universität Musterstadt",
			Street:         "Musterstraße 1",
			City:           "Musterstadt",
			PostalCode:     "12345",
			Country:        "DE",
			VatID:          "DE123456789",
			EndpointID:     "rechnung@uni-musterstadt.de",
			EndpointScheme: "EM",
			Email:          "rechnung@uni-musterstadt.de",
		},
		Buyer: Party{
			Name:       "Stadt Musterstadt",
			Street:     "Rathausplatz 1",
			City:       "Musterstadt",
			PostalCode: "12345",
			Country:    "DE",
		},
		PaymentMeansCode: "58",
		PayeeIBAN:        "DE75512108001245126199",
		PaymentTerms:     "Zahlbar innerhalb von 30 Tagen ohne Abzug.",
		Lines: []Line{
			{LineNo: 1, Description: "Druckleistung Semesterapparat", QuantityMilli: 1000, UnitCode: "C62",
				UnitPriceCents: 28879, NetCents: 28879, VatCategory: "S", VatRateBP: 700},
			{LineNo: 2, Description: "Literaturbeschaffung", QuantityMilli: 2000, UnitCode: "C62",
				UnitPriceCents: 1304, NetCents: 2608, VatCategory: "S", VatRateBP: 1900},
		},
	}
}

func TestGenerateXRechnungGolden(t *testing.T) {
	got, err := GenerateXRechnung(sample())
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_invoice.xml"), []byte(got))
}

// TestGoldenValidatesAgainstXSD validates the golden against the productive
// XRechnung 3.0 UBL schema. It is skipped unless ERP_XRECHNUNG_XSD points at the
// vendor's UBL-Invoice-2.1.xsd (the schema is proprietary and lives outside the
// repo).
func TestGoldenValidatesAgainstXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XRECHNUNG_XSD", filepath.Join("testdata", "golden_invoice.xml"))
}

func TestComputeTotals(t *testing.T) {
	net, vat, gross, groups := ComputeTotals(sample())
	if net != 31487 || vat != 2518 || gross != 34005 {
		t.Fatalf("totals = %d %d %d", net, vat, gross)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].RateBP != 700 || groups[0].TaxableCents != 28879 || groups[0].TaxCents != 2022 {
		t.Fatalf("group 0 = %+v", groups[0])
	}
	if groups[1].RateBP != 1900 || groups[1].TaxableCents != 2608 || groups[1].TaxCents != 496 {
		t.Fatalf("group 1 = %+v", groups[1])
	}
}

func TestLineNetRounding(t *testing.T) {
	cases := []struct{ qty, price, want int64 }{
		{1000, 100, 100},
		{2000, 1304, 2608},
		{1500, 1000, 1500},
		{500, 333, 167}, // 166.5 -> 167 (half away from zero)
		{1, 1, 0},       // 0.001 -> 0
	}
	for _, c := range cases {
		if got := LineNet(c.qty, c.price); got != c.want {
			t.Errorf("LineNet(%d,%d) = %d, want %d", c.qty, c.price, got, c.want)
		}
	}
}

func TestFormatting(t *testing.T) {
	if money(34005) != "340.05" || money(0) != "0.00" || money(-5) != "-0.05" || money(100) != "1.00" {
		t.Errorf("money: %s %s %s %s", money(34005), money(0), money(-5), money(100))
	}
	if qty(1000) != "1" || qty(1500) != "1.5" || qty(1250) != "1.25" || qty(1) != "0.001" {
		t.Errorf("qty: %s %s %s %s", qty(1000), qty(1500), qty(1250), qty(1))
	}
	if rate(1900) != "19" || rate(700) != "7" || rate(750) != "7.5" || rate(0) != "0" {
		t.Errorf("rate: %s %s %s %s", rate(1900), rate(700), rate(750), rate(0))
	}
}

func TestEscape(t *testing.T) {
	if escape(`A&B<C>"D"'E'`) != "A&amp;B&lt;C&gt;&quot;D&quot;&apos;E&apos;" {
		t.Fatalf("escape = %s", escape(`A&B<C>"D"'E'`))
	}
}

func TestValidateErrors(t *testing.T) {
	mut := func(f func(*Invoice)) Invoice {
		inv := sample()
		f(&inv)
		return inv
	}
	cases := []struct {
		name string
		inv  Invoice
		want error
	}{
		{"no number", mut(func(i *Invoice) { i.Number = " " }), ErrNumberRequired},
		{"no date", mut(func(i *Invoice) { i.IssueDate = time.Time{} }), ErrIssueDateRequired},
		{"bad currency", mut(func(i *Invoice) { i.Currency = "EU" }), ErrCurrency},
		{"no seller", mut(func(i *Invoice) { i.Seller.Name = "" }), ErrSellerRequired},
		{"no buyer", mut(func(i *Invoice) { i.Buyer.Name = "" }), ErrBuyerRequired},
		{"no seller city", mut(func(i *Invoice) { i.Seller.City = "" }), ErrPartyCityRequired},
		{"no buyer zip", mut(func(i *Invoice) { i.Buyer.PostalCode = "" }), ErrPartyPostalCode},
		{"bad country", mut(func(i *Invoice) { i.Seller.Country = "DEU" }), ErrPartyCountry},
		{"no seller vat", mut(func(i *Invoice) { i.Seller.VatID = "" }), ErrSellerVATRequired},
		{"no seller email", mut(func(i *Invoice) { i.Seller.Email = "" }), ErrSellerEmailRequired},
		{"no buyer ref", mut(func(i *Invoice) { i.BuyerReference = "" }), ErrBuyerReference},
		{"no payment means", mut(func(i *Invoice) { i.PaymentMeansCode = "" }), ErrPaymentMeansRequired},
		{"no iban", mut(func(i *Invoice) { i.PayeeIBAN = "" }), ErrPayeeIBANRequired},
		{"no lines", mut(func(i *Invoice) { i.Lines = nil }), ErrNoLines},
		{"no line desc", mut(func(i *Invoice) { i.Lines[0].Description = " " }), ErrLineDescription},
		{"zero qty", mut(func(i *Invoice) { i.Lines[0].QuantityMilli = 0 }), ErrLineQuantity},
		{"neg price", mut(func(i *Invoice) { i.Lines[0].UnitPriceCents = -1; i.Lines[0].NetCents = 0 }), ErrLineAmount},
		{"no vat cat", mut(func(i *Invoice) { i.Lines[0].VatCategory = "" }), ErrVATCategoryRequired},
		{"net mismatch", mut(func(i *Invoice) { i.Lines[0].NetCents = 1 }), ErrLineNetMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(c.inv); !errors.Is(err, c.want) {
				t.Fatalf("Validate = %v, want %v", err, c.want)
			}
			if _, err := GenerateXRechnung(c.inv); !errors.Is(err, c.want) {
				t.Fatalf("GenerateXRechnung = %v, want %v", err, c.want)
			}
		})
	}
}

func TestDefaultStrAndUnit(t *testing.T) {
	if defaultStr("", "EM") != "EM" || defaultStr("X", "EM") != "X" {
		t.Fatal("defaultStr")
	}
	if unit("") != "C62" || unit("KGM") != "KGM" {
		t.Fatal("unit")
	}
}
