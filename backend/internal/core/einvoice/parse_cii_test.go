package einvoice

import (
	"errors"
	"strings"
	"testing"
)

// TestParseCIITakesBackWhatGenerateCIIWrites is the round-trip the two halves exist for: what this
// package emits must read back into the model it was built from. It compares the fields that carry
// business meaning, so a writer that drops a term or a reader that mis-paths one shows up here.
func TestParseCIITakesBackWhatGenerateCIIWrites(t *testing.T) {
	want := sample()
	doc, err := GenerateCII(want)
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	got, err := ParseCII(doc)
	if err != nil {
		t.Fatalf("ParseCII: %v", err)
	}

	if got.Number != want.Number || got.TypeCode != want.TypeCode || got.Currency != want.Currency {
		t.Fatalf("header = %q/%q/%q, want %q/%q/%q",
			got.Number, got.TypeCode, got.Currency, want.Number, want.TypeCode, want.Currency)
	}
	if !got.IssueDate.Equal(want.IssueDate) {
		t.Fatalf("issue date = %s, want %s", got.IssueDate, want.IssueDate)
	}
	if got.BuyerReference != want.BuyerReference {
		t.Fatalf("buyer reference = %q, want %q", got.BuyerReference, want.BuyerReference)
	}
	if got.PaymentMeansCode != want.PaymentMeansCode || got.PayeeIBAN != want.PayeeIBAN {
		t.Fatalf("payment = %q/%q, want %q/%q", got.PaymentMeansCode, got.PayeeIBAN, want.PaymentMeansCode, want.PayeeIBAN)
	}
	if got.PaymentTerms != want.PaymentTerms {
		t.Fatalf("payment terms = %q, want %q", got.PaymentTerms, want.PaymentTerms)
	}
	for name, pair := range map[string][2]Party{"seller": {got.Seller, want.Seller}, "buyer": {got.Buyer, want.Buyer}} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %+v, want %+v", name, pair[0], pair[1])
		}
	}
	if len(got.Lines) != len(want.Lines) {
		t.Fatalf("lines = %d, want %d", len(got.Lines), len(want.Lines))
	}
	for i := range want.Lines {
		if got.Lines[i] != want.Lines[i] {
			t.Errorf("line %d = %+v, want %+v", i+1, got.Lines[i], want.Lines[i])
		}
	}
	// The same figures the model computes from what was read.
	gotNet, gotVAT, gotGross, _ := ComputeTotals(got)
	wantNet, wantVAT, wantGross, _ := ComputeTotals(want)
	if gotNet != wantNet || gotVAT != wantVAT || gotGross != wantGross {
		t.Fatalf("totals = %d/%d/%d, want %d/%d/%d", gotNet, gotVAT, gotGross, wantNet, wantVAT, wantGross)
	}
}

// A document this reader cannot make sense of is refused, not half-read: the error names the fault,
// and nothing is returned that a caller could mistake for an invoice.
func TestParseCIIRefusesDocumentsItCannotRead(t *testing.T) {
	good, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	cases := []struct {
		name string
		doc  string
		want error
	}{
		{"empty", "", ErrMalformed},
		{"not xml", "kaputt", ErrMalformed},
		{"a UBL invoice", `<?xml version="1.0"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"><ID>RE-1</ID></Invoice>`, ErrMalformed},
		{"no number", strings.Replace(good, "<ram:ID>RE-2026-0001</ram:ID>", "<ram:ID></ram:ID>", 1), ErrMalformed},
		{"no issue date", strings.Replace(good, `<udt:DateTimeString format="102">20261026</udt:DateTimeString>`, `<udt:DateTimeString format="102"></udt:DateTimeString>`, 1), ErrBadValue},
		{"a date format this reader does not know", strings.Replace(good, `format="102"`, `format="610"`, 1), ErrBadValue},
		{"a line id that is not a number", strings.Replace(good, "<ram:LineID>2</ram:LineID>", "<ram:LineID>zwei</ram:LineID>", 1), ErrBadValue},
		{"a line whose net does not match quantity x price", strings.Replace(good, "<ram:LineTotalAmount>26.08</ram:LineTotalAmount>", "<ram:LineTotalAmount>26.09</ram:LineTotalAmount>", 1), ErrLineNetMismatch},
		{"no lines at all", dropLines(good), ErrNoLines},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv, err := ParseCII(c.doc)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if inv.Number != "" || len(inv.Lines) != 0 {
				t.Fatalf("a refused document returned an invoice: %+v", inv)
			}
		})
	}
}

// Only the VAT registration is a tax id: another scheme under SpecifiedTaxRegistration is a different
// identifier and must not be read into the VAT field.
func TestParseCIIReadsOnlyTheVatRegistrationAsTaxID(t *testing.T) {
	party := func(scheme, id string) ciiParty {
		var p ciiParty
		p.TaxRegistration.ID = ciiSchemeID{Scheme: scheme, Value: id}
		return p
	}
	if got := toDomainCIIParty(party("VA", "DE123456789")).VatID; got != "DE123456789" {
		t.Fatalf("VA id = %q, want it adopted", got)
	}
	if got := toDomainCIIParty(party("FC", "HRB-12345")).VatID; got != "" {
		t.Fatalf("FC id = %q, want it ignored", got)
	}
	// A registration without a scheme is still the VAT id: some senders omit the attribute.
	if got := toDomainCIIParty(party("", "DE123456789")).VatID; got != "DE123456789" {
		t.Fatalf("unschemed id = %q, want it adopted", got)
	}

	// And end to end: a document whose seller carries only a commercial register number is refused,
	// because the seller VAT id is required — the refusal is what proves nothing was adopted.
	doc, err := GenerateCII(sample())
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}
	other := strings.Replace(doc, `<ram:ID schemeID="VA">DE123456789</ram:ID>`, `<ram:ID schemeID="FC">HRB-12345</ram:ID>`, 1)
	if _, err := ParseCII(other); !errors.Is(err, ErrSellerVATRequired) {
		t.Fatalf("err = %v, want ErrSellerVATRequired", err)
	}
}

// dropLines removes every trade line from a rendered document, leaving a well-formed invoice without
// one — the shape Validate refuses with ErrNoLines.
func dropLines(doc string) string {
	start := strings.Index(doc, "<ram:IncludedSupplyChainTradeLineItem>")
	if start < 0 {
		return doc
	}
	end := strings.LastIndex(doc, "</ram:IncludedSupplyChainTradeLineItem>")
	if end < 0 {
		return doc
	}
	return doc[:start] + doc[end+len("</ram:IncludedSupplyChainTradeLineItem>"):]
}

// An inbound document is read without the caller knowing its syntax: both are the same invoice, and
// the root element says which one arrived.
func TestParseReceivedReadsBothSyntaxes(t *testing.T) {
	want := sample()
	ubl, err := GenerateXRechnung(want)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	cii, err := GenerateCII(want)
	if err != nil {
		t.Fatalf("GenerateCII: %v", err)
	}

	for name, doc := range map[string]string{"UBL": ubl, "CII": cii} {
		got, err := ParseReceived(doc)
		if err != nil {
			t.Fatalf("%s: ParseReceived: %v", name, err)
		}
		if got.Number != want.Number || len(got.Lines) != len(want.Lines) {
			t.Fatalf("%s: read %q with %d lines, want %q with %d", name, got.Number, len(got.Lines), want.Number, len(want.Lines))
		}
		// The same model from either syntax: the totals are the model's, not the syntax's.
		gotNet, _, gotGross, _ := ComputeTotals(got)
		wantNet, _, wantGross, _ := ComputeTotals(want)
		if gotNet != wantNet || gotGross != wantGross {
			t.Fatalf("%s: totals %d/%d, want %d/%d", name, gotNet, gotGross, wantNet, wantGross)
		}
	}
}

// Anything that is not one of the two syntaxes is refused, including a container.
func TestParseReceivedRefusesOtherDocuments(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":       "",
		"a container": `<?xml version="1.0"?><rfc:CrossIndustryInvoiceContainer></rfc:CrossIndustryInvoiceContainer>`,
		"a prologue":  `<?xml version="1.0" encoding="UTF-8"?><!-- a comment -->`,
	} {
		if _, err := ParseReceived(doc); !errors.Is(err, ErrMalformed) {
			t.Fatalf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}
