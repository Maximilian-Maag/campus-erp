package einvoice

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseXRechnungRoundTrip(t *testing.T) {
	doc, err := GenerateXRechnung(sample())
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	got, err := ParseXRechnung(doc)
	if err != nil {
		t.Fatalf("ParseXRechnung: %v", err)
	}
	want := sample()
	if got.Number != want.Number || got.TypeCode != want.TypeCode || got.Currency != want.Currency {
		t.Fatalf("header = %+v", got)
	}
	if !got.IssueDate.Equal(want.IssueDate) {
		t.Fatalf("issue date = %v", got.IssueDate)
	}
	if got.BuyerReference != want.BuyerReference || got.PaymentMeansCode != want.PaymentMeansCode || got.PayeeIBAN != want.PayeeIBAN {
		t.Fatalf("payment = %+v", got)
	}
	if got.Seller != want.Seller {
		t.Fatalf("seller = %+v, want %+v", got.Seller, want.Seller)
	}
	if got.Buyer != want.Buyer {
		t.Fatalf("buyer = %+v, want %+v", got.Buyer, want.Buyer)
	}
	if got.PaymentTerms != want.PaymentTerms {
		t.Fatalf("terms = %q", got.PaymentTerms)
	}
	if len(got.Lines) != len(want.Lines) {
		t.Fatalf("lines = %d", len(got.Lines))
	}
	for i := range got.Lines {
		if got.Lines[i] != want.Lines[i] {
			t.Fatalf("line %d = %+v, want %+v", i, got.Lines[i], want.Lines[i])
		}
	}
}

func TestParseXRechnungErrors(t *testing.T) {
	valid, _ := GenerateXRechnung(sample())

	cases := []struct {
		name string
		doc  string
		want error
	}{
		{"not xml", "<not-xml", ErrMalformed},
		{"missing id", strings.Replace(valid, "<cbc:ID>RE-2026-0001</cbc:ID>", "<cbc:ID></cbc:ID>", 1), ErrMalformed},
		{"bad date", strings.Replace(valid, "<cbc:IssueDate>2026-10-26</cbc:IssueDate>", "<cbc:IssueDate>26.10.2026</cbc:IssueDate>", 1), ErrBadValue},
		{"bad amount", strings.Replace(valid, `<cbc:PriceAmount currencyID="EUR">288.79</cbc:PriceAmount>`, `<cbc:PriceAmount currencyID="EUR">abc</cbc:PriceAmount>`, 1), ErrBadValue},
		{"bad percent", strings.ReplaceAll(valid, "<cbc:Percent>7</cbc:Percent>", "<cbc:Percent>seven</cbc:Percent>"), ErrBadValue},
		{"bad quantity", strings.Replace(valid, `<cbc:InvoicedQuantity unitCode="C62">1</cbc:InvoicedQuantity>`, `<cbc:InvoicedQuantity unitCode="C62">x</cbc:InvoicedQuantity>`, 1), ErrBadValue},
		{"validation fails", strings.Replace(valid, "<cbc:BuyerReference>04011000-12345-03</cbc:BuyerReference>", "<cbc:BuyerReference></cbc:BuyerReference>", 1), ErrBuyerReference},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseXRechnung(c.doc); !errors.Is(err, c.want) {
				t.Fatalf("ParseXRechnung = %v, want %v", err, c.want)
			}
		})
	}
}

func TestParseDecimalHelpers(t *testing.T) {
	if v, err := parseMoney("340.05"); err != nil || v != 34005 {
		t.Errorf("parseMoney 340.05 = %d %v", v, err)
	}
	if v, err := parseMoney("100"); err != nil || v != 10000 {
		t.Errorf("parseMoney 100 = %d %v", v, err)
	}
	if v, err := parseMoney("-0.05"); err != nil || v != -5 {
		t.Errorf("parseMoney -0.05 = %d %v", v, err)
	}
	if v, err := parseQuantityMilli("1.5"); err != nil || v != 1500 {
		t.Errorf("parseQuantityMilli 1.5 = %d %v", v, err)
	}
	if v, err := parseQuantityMilli("0.001"); err != nil || v != 1 {
		t.Errorf("parseQuantityMilli 0.001 = %d %v", v, err)
	}
	if v, err := parsePercentBP("19"); err != nil || v != 1900 {
		t.Errorf("parsePercentBP 19 = %d %v", v, err)
	}
	if v, err := parsePercentBP("7.5"); err != nil || v != 750 {
		t.Errorf("parsePercentBP 7.5 = %d %v", v, err)
	}
	if _, err := parseMoney(""); !errors.Is(err, ErrBadValue) {
		t.Errorf("parseMoney empty should fail")
	}
	if _, err := parseMoney("1.2.3"); !errors.Is(err, ErrBadValue) {
		t.Errorf("parseMoney 1.2.3 should fail")
	}
	// A date-only issue date is required.
	if _, err := ParseXRechnung("<Invoice><ID>x</ID><IssueDate></IssueDate></Invoice>"); !errors.Is(err, ErrMalformed) {
		t.Errorf("missing date should be malformed")
	}
	_ = time.Now
}
