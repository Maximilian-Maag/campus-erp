package einvoice

import (
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

// The CII rendering is pinned byte-for-byte: it is one model with two syntaxes, and a change that
// touches only this one must be visible rather than silently shipped to a counterpart's parser.
func TestGenerateCIIGolden(t *testing.T) {
	got, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_invoice_cii.xml"), []byte(got))
}

// The same invoice rendered twice is the same document: no timestamp but the invoice's own date, so
// a golden file is a real guard and not a coin flip.
func TestGenerateCIIIsDeterministic(t *testing.T) {
	first, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	second, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	if first != second {
		t.Fatal("two renderings of one invoice differ")
	}
}

// No CII schema is committed (it is the standard's, and the counterpart's validation is the real
// one), so the guard is the business terms the document must carry — the same ones the UBL
// rendering is checked for.
func TestGenerateCIICarriesTheBusinessTerms(t *testing.T) {
	inv := sample()
	doc, err := GenerateCII(inv)
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	net, vat, gross, groups := ComputeTotals(inv)

	wants := []string{
		`xmlns:rsm="` + nsRsm + `"`,
		"<rsm:CrossIndustryInvoice\n",
		"<ram:ID>" + CIIGuideline + "</ram:ID>",             // BT-24, the guideline
		"<ram:ID>" + inv.Number + "</ram:ID>",               // BT-1
		"<ram:TypeCode>" + inv.TypeCode + "</ram:TypeCode>", // BT-3
		`<udt:DateTimeString format="102">20261026</udt:DateTimeString>`,
		"<ram:BuyerReference>" + inv.BuyerReference + "</ram:BuyerReference>", // BT-10
		"<ram:GrandTotalAmount>" + money(gross) + "</ram:GrandTotalAmount>",
		"<ram:DuePayableAmount>" + money(gross) + "</ram:DuePayableAmount>",
		"<ram:TaxBasisTotalAmount>" + money(net) + "</ram:TaxBasisTotalAmount>",
		"<ram:LineTotalAmount>" + money(net) + "</ram:LineTotalAmount>",
		`<ram:TaxTotalAmount currencyID="EUR">` + money(vat) + "</ram:TaxTotalAmount>",
		"<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>",
		"</rsm:CrossIndustryInvoice>",
	}
	for _, want := range wants {
		if !strings.Contains(doc, want) {
			t.Fatalf("the CII document is missing %q", want)
		}
	}

	// The seller carries what a German public-sector invoice demands and the buyer does not.
	if !strings.Contains(doc, `<ram:ID schemeID="VA">`+inv.Seller.VatID+"</ram:ID>") {
		t.Fatal("the seller VAT id (BR-DE-16) is missing or unnamed")
	}
	if !strings.Contains(doc, "<ram:URIID>"+inv.Seller.Email+"</ram:URIID>") {
		t.Fatal("the seller contact e-mail (BR-DE-7) is missing")
	}
	// Every VAT group is stated with its basis, its tax and its rate.
	for _, g := range groups {
		want := "<ram:CalculatedAmount>" + money(g.TaxCents) + "</ram:CalculatedAmount>"
		if !strings.Contains(doc, want) {
			t.Fatalf("VAT group %s/%d is missing its tax: %q", g.Category, g.RateBP, want)
		}
		if !strings.Contains(doc, "<ram:BasisAmount>"+money(g.TaxableCents)+"</ram:BasisAmount>") {
			t.Fatalf("VAT group %s/%d is missing its basis", g.Category, g.RateBP)
		}
		// The header amounts come before the summation, as the standard's sequence has them.
		groupAt := strings.Index(doc, "<ram:CalculatedAmount>"+money(g.TaxCents))
		sumAt := strings.Index(doc, "<ram:SpecifiedTradeSettlementHeaderMonetarySummation>")
		if groupAt < 0 || sumAt < 0 || groupAt > sumAt {
			t.Fatalf("VAT group %s/%d is not before the monetary summation", g.Category, g.RateBP)
		}
	}

	// Every line is there with its quantity, its price and its own settlement.
	for _, l := range inv.Lines {
		for _, want := range []string{
			"<ram:LineID>" + itoa(int64(l.LineNo)) + "</ram:LineID>",
			`<ram:BilledQuantity unitCode="` + l.UnitCode + `">` + qty(l.QuantityMilli) + "</ram:BilledQuantity>",
			"<ram:ChargeAmount>" + money(l.UnitPriceCents) + "</ram:ChargeAmount>",
			"<ram:Name>" + escape(l.Description) + "</ram:Name>",
		} {
			if !strings.Contains(doc, want) {
				t.Fatalf("line %d is missing %q", l.LineNo, want)
			}
		}
	}
	// A line without a unit code falls back to C62, exactly as the UBL rendering does.
	if strings.Contains(doc, `unitCode=""`) {
		t.Fatal("a line rendered an empty unit code")
	}
}

// No schema is committed, so the first thing the document must be is *well formed*: a rendering
// helper that drops a closing tag's slash is the kind of mistake a string assertion misses and a
// counterpart's parser rejects outright.
func TestGenerateCIIIsWellFormed(t *testing.T) {
	doc, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	dec := xml.NewDecoder(strings.NewReader(doc))
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("the CII document is not well formed: %v", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				t.Fatal("the CII document closes an element that was never opened")
			}
		}
	}
	if depth != 0 {
		t.Fatalf("the CII document leaves %d elements open", depth)
	}
}

// The rules are the model's, not the syntax's: an invoice the UBL rendering refuses is refused here
// too, with the same error.
func TestGenerateCIIRefusesAnInvalidInvoice(t *testing.T) {
	inv := sample()
	inv.Number = "  "
	if _, err := GenerateCII(inv); err == nil {
		t.Fatal("an invoice without a number was rendered")
	}
	inv = sample()
	inv.PaymentMeansCode = "999"
	if _, err := GenerateCII(inv); err == nil {
		t.Fatal("an invoice with a payment means code EN 16931 does not allow was rendered")
	}
}

// An invoice without payment terms states none, and one with them states them: the element is
// optional in the sequence and must not appear empty.
func TestGenerateCIIPaymentTerms(t *testing.T) {
	inv := sample()
	inv.PaymentTerms = ""
	doc, err := GenerateCII(inv)
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	if strings.Contains(doc, "SpecifiedTradePaymentTerms") {
		t.Fatal("an invoice without payment terms rendered the element")
	}
	inv.PaymentTerms = "Zahlbar innerhalb von 30 Tagen."
	doc, err = GenerateCII(inv)
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	if !strings.Contains(doc, "<ram:Description>"+inv.PaymentTerms+"</ram:Description>") {
		t.Fatal("the payment terms are missing")
	}
}
