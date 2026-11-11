package einvoice

import (
	"strings"
)

// ZUGFeRD / Factur-X syntax: the same canonical invoice as the UBL document, written as UN/CEFACT
// Cross Industry Invoice (CII) (docs/09).
//
// XRechnung (UBL) is what a German public body *receives*; ZUGFeRD is what a supplier is asked for
// when the counterpart prefers a hybrid file, and both carry the EN 16931 business terms. The model
// is shared, so this file is a second rendering of it — not a second model.
//
// What this is: the **CII data part**. A ZUGFeRD file proper is a PDF/A-3 with this XML embedded as
// the `factur-x.xml` attachment; embedding it is a PDF/A job of its own (docs/09).
const (
	// CIIGuideline is the EN 16931 core guideline (BT-24). A ZUGFeRD profile id — for instance
	// `urn:cen.eu:en16931:2017#compliant#urn:factur-x.eu:1p0:en16931` — is a value of the same
	// parameter, so a counterpart that demands one states it rather than this file guessing.
	CIIGuideline = "urn:cen.eu:en16931:2017"
	// CIIConformanceLevel names the profile in the header (the ZUGFeRD `ConformanceLevel`); the
	// EN 16931 profile is what the model enforces.
	CIIConformanceLevel = "EN 16931"
)

// CII namespaces, as the standard fixes them.
const (
	nsRsm = "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100"
	nsRam = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100"
	nsUdt = "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100"
	nsQdt = "urn:un:unece:uncefact:data:standard:QualifiedDataType:100"
)

// GenerateCII validates the invoice and returns a deterministic ZUGFeRD/Factur-X (CII) document.
// It runs the same validation the UBL rendering does, so a document this system refuses to emit as
// XRechnung is refused here too — the rules are the model's, not the syntax's.
func GenerateCII(inv Invoice) (string, error) {
	if err := Validate(inv); err != nil {
		return "", err
	}
	net, vat, gross, groups := ComputeTotals(inv)
	ccy := inv.Currency

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<rsm:CrossIndustryInvoice" + "\n")
	b.WriteString(`    xmlns:rsm="` + nsRsm + `"` + "\n")
	b.WriteString(`    xmlns:qdt="` + nsQdt + `"` + "\n")
	b.WriteString(`    xmlns:ram="` + nsRam + `"` + "\n")
	b.WriteString(`    xmlns:udt="` + nsUdt + `">` + "\n")

	b.WriteString(cii(1, "rsm:ExchangedDocumentContext"))
	b.WriteString(cii(2, "ram:BusinessProcessSpecifiedDocumentContextParameter"))
	b.WriteString(ciiText(3, "ram:ID", "A1"))
	b.WriteString(cii(2, "/ram:BusinessProcessSpecifiedDocumentContextParameter"))
	b.WriteString(cii(2, "ram:GuidelineSpecifiedDocumentContextParameter"))
	b.WriteString(ciiText(3, "ram:ID", CIIGuideline))
	b.WriteString(cii(2, "/ram:GuidelineSpecifiedDocumentContextParameter"))
	b.WriteString(cii(1, "/rsm:ExchangedDocumentContext"))

	b.WriteString(cii(1, "rsm:ExchangedDocument"))
	b.WriteString(ciiText(2, "ram:ID", inv.Number))
	b.WriteString(ciiText(2, "ram:TypeCode", inv.TypeCode))
	b.WriteString(cii(2, "ram:IssueDateTime"))
	b.WriteString(ciiAttr(3, "udt:DateTimeString", "format", "102", inv.IssueDate.Format("20060102")))
	b.WriteString(cii(2, "/ram:IssueDateTime"))
	b.WriteString(cii(1, "/rsm:ExchangedDocument"))

	b.WriteString(cii(1, "rsm:SupplyChainTradeTransaction"))
	for _, l := range inv.Lines {
		b.WriteString(cii(2, "ram:IncludedSupplyChainTradeLineItem"))
		b.WriteString(cii(3, "ram:AssociatedDocumentLineDocument"))
		b.WriteString(ciiText(4, "ram:LineID", itoa(int64(l.LineNo))))
		b.WriteString(cii(3, "/ram:AssociatedDocumentLineDocument"))
		b.WriteString(cii(3, "ram:SpecifiedTradeProduct"))
		b.WriteString(ciiText(4, "ram:Name", l.Description))
		b.WriteString(cii(3, "/ram:SpecifiedTradeProduct"))
		b.WriteString(cii(3, "ram:SpecifiedLineTradeAgreement"))
		b.WriteString(cii(4, "ram:NetPriceProductTradePrice"))
		b.WriteString(ciiText(5, "ram:ChargeAmount", money(l.UnitPriceCents)))
		b.WriteString(cii(4, "/ram:NetPriceProductTradePrice"))
		b.WriteString(cii(3, "/ram:SpecifiedLineTradeAgreement"))
		b.WriteString(cii(3, "ram:SpecifiedLineTradeDelivery"))
		b.WriteString(ciiAttr(4, "ram:BilledQuantity", "unitCode", unit(l.UnitCode), qty(l.QuantityMilli)))
		b.WriteString(cii(3, "/ram:SpecifiedLineTradeDelivery"))
		b.WriteString(cii(3, "ram:SpecifiedLineTradeSettlement"))
		b.WriteString(tradeTax(4, l.NetCents, 0, l.VatCategory, l.VatRateBP, false))
		b.WriteString(cii(4, "ram:SpecifiedTradeSettlementLineMonetarySummation"))
		b.WriteString(ciiText(5, "ram:LineTotalAmount", money(l.NetCents)))
		b.WriteString(cii(4, "/ram:SpecifiedTradeSettlementLineMonetarySummation"))
		b.WriteString(cii(3, "/ram:SpecifiedLineTradeSettlement"))
		b.WriteString(cii(2, "/ram:IncludedSupplyChainTradeLineItem"))
	}

	b.WriteString(cii(2, "ram:ApplicableHeaderTradeAgreement"))
	b.WriteString(ciiText(3, "ram:BuyerReference", inv.BuyerReference))
	b.WriteString(tradeParty(3, "ram:SellerTradeParty", inv.Seller, true))
	b.WriteString(tradeParty(3, "ram:BuyerTradeParty", inv.Buyer, false))
	b.WriteString(cii(2, "/ram:ApplicableHeaderTradeAgreement"))

	// CII requires the delivery group even when it carries nothing: a public buyer's invoice states
	// no delivery date here, and the element is part of the sequence.
	b.WriteString(cii(2, "ram:ApplicableHeaderTradeDelivery"))
	b.WriteString(cii(2, "/ram:ApplicableHeaderTradeDelivery"))

	b.WriteString(cii(2, "ram:ApplicableHeaderTradeSettlement"))
	b.WriteString(ciiText(3, "ram:InvoiceCurrencyCode", ccy))
	b.WriteString(cii(3, "ram:SpecifiedTradeSettlementPaymentMeans"))
	b.WriteString(ciiText(4, "ram:TypeCode", inv.PaymentMeansCode))
	b.WriteString(cii(4, "ram:PayeePartyCreditorFinancialAccount"))
	b.WriteString(ciiText(5, "ram:IBANID", inv.PayeeIBAN))
	b.WriteString(cii(4, "/ram:PayeePartyCreditorFinancialAccount"))
	b.WriteString(cii(3, "/ram:SpecifiedTradeSettlementPaymentMeans"))
	for _, g := range groups {
		b.WriteString(tradeTax(3, g.TaxableCents, g.TaxCents, g.Category, g.RateBP, true))
	}
	if strings.TrimSpace(inv.PaymentTerms) != "" {
		b.WriteString(cii(3, "ram:SpecifiedTradePaymentTerms"))
		b.WriteString(ciiText(4, "ram:Description", inv.PaymentTerms))
		b.WriteString(cii(3, "/ram:SpecifiedTradePaymentTerms"))
	}
	b.WriteString(cii(3, "ram:SpecifiedTradeSettlementHeaderMonetarySummation"))
	b.WriteString(ciiText(4, "ram:LineTotalAmount", money(net)))
	b.WriteString(ciiText(4, "ram:TaxBasisTotalAmount", money(net)))
	b.WriteString(ciiAttr(4, "ram:TaxTotalAmount", "currencyID", ccy, money(vat)))
	b.WriteString(ciiText(4, "ram:GrandTotalAmount", money(gross)))
	b.WriteString(ciiText(4, "ram:DuePayableAmount", money(gross)))
	b.WriteString(cii(3, "/ram:SpecifiedTradeSettlementHeaderMonetarySummation"))
	b.WriteString(cii(2, "/ram:ApplicableHeaderTradeSettlement"))
	b.WriteString(cii(1, "/rsm:SupplyChainTradeTransaction"))

	b.WriteString("</rsm:CrossIndustryInvoice>\n")
	return b.String(), nil
}

// tradeTax writes an ApplicableTradeTax. On a line it carries the category and the rate; in the
// header it additionally carries what the group is worth — the line's own tax is derived by the
// header, which is why the amounts are only written where they are stated.
func tradeTax(level int, basisCents, taxCents int64, category string, rateBP int64, withAmounts bool) string {
	var b strings.Builder
	b.WriteString(cii(level, "ram:ApplicableTradeTax"))
	if withAmounts {
		b.WriteString(ciiText(level+1, "ram:CalculatedAmount", money(taxCents)))
	}
	b.WriteString(ciiText(level+1, "ram:TypeCode", "VAT"))
	if withAmounts {
		b.WriteString(ciiText(level+1, "ram:BasisAmount", money(basisCents)))
	}
	b.WriteString(ciiText(level+1, "ram:CategoryCode", category))
	b.WriteString(ciiText(level+1, "ram:RateApplicablePercent", rate(rateBP)))
	b.WriteString(cii(level, "/ram:ApplicableTradeTax"))
	return b.String()
}

// tradeParty writes a SellerTradeParty or BuyerTradeParty. The seller carries what a German
// public-sector invoice demands and the buyer does not: the VAT id (BR-DE-16) and the contact
// e-mail (BR-DE-7).
func tradeParty(level int, tag string, p Party, seller bool) string {
	var b strings.Builder
	b.WriteString(cii(level, tag))
	b.WriteString(ciiText(level+1, "ram:Name", p.Name))
	if strings.TrimSpace(p.EndpointID) != "" {
		b.WriteString(cii(level+1, "ram:URIUniversalCommunication"))
		b.WriteString(ciiAttr(level+2, "ram:URIID", "schemeID", defaultStr(p.EndpointScheme, "EM"), p.EndpointID))
		b.WriteString(cii(level+1, "/ram:URIUniversalCommunication"))
	}
	b.WriteString(cii(level+1, "ram:PostalTradeAddress"))
	if strings.TrimSpace(p.Street) != "" {
		b.WriteString(ciiText(level+2, "ram:LineOne", p.Street))
	}
	b.WriteString(ciiText(level+2, "ram:PostcodeCode", p.PostalCode))
	b.WriteString(ciiText(level+2, "ram:CityName", p.City))
	b.WriteString(ciiText(level+2, "ram:CountryID", p.Country))
	b.WriteString(cii(level+1, "/ram:PostalTradeAddress"))
	if seller && strings.TrimSpace(p.VatID) != "" {
		b.WriteString(cii(level+1, "ram:SpecifiedTaxRegistration"))
		b.WriteString(ciiAttr(level+2, "ram:ID", "schemeID", "VA", p.VatID))
		b.WriteString(cii(level+1, "/ram:SpecifiedTaxRegistration"))
	}
	if seller && strings.TrimSpace(p.Email) != "" {
		b.WriteString(cii(level+1, "ram:DefinedTradeContact"))
		b.WriteString(ciiText(level+2, "ram:PersonName", p.Name))
		b.WriteString(cii(level+2, "ram:EmailURIUniversalCommunication"))
		b.WriteString(ciiText(level+3, "ram:URIID", p.Email))
		b.WriteString(cii(level+2, "/ram:EmailURIUniversalCommunication"))
		b.WriteString(cii(level+1, "/ram:DefinedTradeContact"))
	}
	b.WriteString(cii(level, "/"+tag))
	return b.String()
}

// cii writes an opening tag; cii with a leading "/" writes a closing one. Both helpers exist so the
// element sequence reads the way the standard's own examples read.
func cii(level int, tag string) string {
	if strings.HasPrefix(tag, "/") {
		return ind(level) + "</" + tag[1:] + ">\n"
	}
	return ind(level) + "<" + tag + ">\n"
}

func ciiText(level int, tag, value string) string {
	return ind(level) + "<" + tag + ">" + escape(value) + "</" + tag + ">\n"
}

func ciiAttr(level int, tag, attr, attrValue, value string) string {
	return ind(level) + "<" + tag + " " + attr + `="` + escape(attrValue) + `">` + escape(value) + "</" + tag + ">\n"
}
