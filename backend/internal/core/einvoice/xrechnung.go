package einvoice

import (
	"strings"
)

// XRechnung 3.0 identification (see the vendor's XRechnung 3.0 example).
const (
	CustomizationID = "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0"
	ProfileID       = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
)

// GenerateXRechnung validates the invoice and returns a deterministic XRechnung
// 3.0 (UBL 2.1) document.
func GenerateXRechnung(inv Invoice) (string, error) {
	if err := Validate(inv); err != nil {
		return "", err
	}
	net, vat, gross, groups := ComputeTotals(inv)
	ccy := inv.Currency

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ubl:Invoice xmlns:ubl="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"` + "\n")
	b.WriteString(`             xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"` + "\n")
	b.WriteString(`             xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">` + "\n")
	b.WriteString(el(1, "cbc:CustomizationID", CustomizationID))
	b.WriteString(el(1, "cbc:ProfileID", ProfileID))
	b.WriteString(el(1, "cbc:ID", inv.Number))
	b.WriteString(el(1, "cbc:IssueDate", inv.IssueDate.Format("2006-01-02")))
	b.WriteString(el(1, "cbc:InvoiceTypeCode", inv.TypeCode))
	b.WriteString(el(1, "cbc:DocumentCurrencyCode", ccy))
	b.WriteString(el(1, "cbc:BuyerReference", inv.BuyerReference))

	b.WriteString(party(1, "cac:AccountingSupplierParty", inv.Seller, true, ccy))
	b.WriteString(party(1, "cac:AccountingCustomerParty", inv.Buyer, false, ccy))

	b.WriteString(ind(1) + "<cac:PaymentMeans>\n")
	b.WriteString(el(2, "cbc:PaymentMeansCode", inv.PaymentMeansCode))
	b.WriteString(ind(2) + "<cac:PayeeFinancialAccount>\n")
	b.WriteString(el(3, "cbc:ID", inv.PayeeIBAN))
	b.WriteString(ind(2) + "</cac:PayeeFinancialAccount>\n")
	b.WriteString(ind(1) + "</cac:PaymentMeans>\n")

	if strings.TrimSpace(inv.PaymentTerms) != "" {
		b.WriteString(ind(1) + "<cac:PaymentTerms>\n")
		b.WriteString(el(2, "cbc:Note", inv.PaymentTerms))
		b.WriteString(ind(1) + "</cac:PaymentTerms>\n")
	}

	b.WriteString(ind(1) + "<cac:TaxTotal>\n")
	b.WriteString(elAttr(2, "cbc:TaxAmount", "currencyID", ccy, money(vat)))
	for _, g := range groups {
		b.WriteString(ind(2) + "<cac:TaxSubtotal>\n")
		b.WriteString(elAttr(3, "cbc:TaxableAmount", "currencyID", ccy, money(g.TaxableCents)))
		b.WriteString(elAttr(3, "cbc:TaxAmount", "currencyID", ccy, money(g.TaxCents)))
		b.WriteString(ind(3) + "<cac:TaxCategory>\n")
		b.WriteString(el(4, "cbc:ID", g.Category))
		b.WriteString(el(4, "cbc:Percent", rate(g.RateBP)))
		b.WriteString(ind(4) + "<cac:TaxScheme>\n")
		b.WriteString(el(5, "cbc:ID", "VAT"))
		b.WriteString(ind(4) + "</cac:TaxScheme>\n")
		b.WriteString(ind(3) + "</cac:TaxCategory>\n")
		b.WriteString(ind(2) + "</cac:TaxSubtotal>\n")
	}
	b.WriteString(ind(1) + "</cac:TaxTotal>\n")

	b.WriteString(ind(1) + "<cac:LegalMonetaryTotal>\n")
	b.WriteString(elAttr(2, "cbc:LineExtensionAmount", "currencyID", ccy, money(net)))
	b.WriteString(elAttr(2, "cbc:TaxExclusiveAmount", "currencyID", ccy, money(net)))
	b.WriteString(elAttr(2, "cbc:TaxInclusiveAmount", "currencyID", ccy, money(gross)))
	b.WriteString(elAttr(2, "cbc:PayableAmount", "currencyID", ccy, money(gross)))
	b.WriteString(ind(1) + "</cac:LegalMonetaryTotal>\n")

	for _, l := range inv.Lines {
		b.WriteString(ind(1) + "<cac:InvoiceLine>\n")
		b.WriteString(el(2, "cbc:ID", itoa(int64(l.LineNo))))
		b.WriteString(elAttr(2, "cbc:InvoicedQuantity", "unitCode", unit(l.UnitCode), qty(l.QuantityMilli)))
		b.WriteString(elAttr(2, "cbc:LineExtensionAmount", "currencyID", ccy, money(l.NetCents)))
		b.WriteString(ind(2) + "<cac:Item>\n")
		b.WriteString(el(3, "cbc:Name", l.Description))
		b.WriteString(ind(3) + "<cac:ClassifiedTaxCategory>\n")
		b.WriteString(el(4, "cbc:ID", l.VatCategory))
		b.WriteString(el(4, "cbc:Percent", rate(l.VatRateBP)))
		b.WriteString(ind(4) + "<cac:TaxScheme>\n")
		b.WriteString(el(5, "cbc:ID", "VAT"))
		b.WriteString(ind(4) + "</cac:TaxScheme>\n")
		b.WriteString(ind(3) + "</cac:ClassifiedTaxCategory>\n")
		b.WriteString(ind(2) + "</cac:Item>\n")
		b.WriteString(ind(2) + "<cac:Price>\n")
		b.WriteString(elAttr(3, "cbc:PriceAmount", "currencyID", ccy, money(l.UnitPriceCents)))
		b.WriteString(ind(2) + "</cac:Price>\n")
		b.WriteString(ind(1) + "</cac:InvoiceLine>\n")
	}

	b.WriteString("</ubl:Invoice>\n")
	return b.String(), nil
}

func party(level int, tag string, p Party, seller bool, _ string) string {
	var b strings.Builder
	b.WriteString(ind(level) + "<" + tag + ">\n")
	b.WriteString(ind(level+1) + "<cac:Party>\n")
	if strings.TrimSpace(p.EndpointID) != "" {
		b.WriteString(elAttr(level+2, "cbc:EndpointID", "schemeID", defaultStr(p.EndpointScheme, "EM"), p.EndpointID))
	}
	b.WriteString(ind(level+2) + "<cac:PartyName>\n")
	b.WriteString(el(level+3, "cbc:Name", p.Name))
	b.WriteString(ind(level+2) + "</cac:PartyName>\n")
	b.WriteString(ind(level+2) + "<cac:PostalAddress>\n")
	b.WriteString(el(level+3, "cbc:StreetName", p.Street))
	b.WriteString(el(level+3, "cbc:CityName", p.City))
	b.WriteString(el(level+3, "cbc:PostalZone", p.PostalCode))
	b.WriteString(ind(level+3) + "<cac:Country>\n")
	b.WriteString(el(level+4, "cbc:IdentificationCode", p.Country))
	b.WriteString(ind(level+3) + "</cac:Country>\n")
	b.WriteString(ind(level+2) + "</cac:PostalAddress>\n")
	if seller && strings.TrimSpace(p.VatID) != "" {
		b.WriteString(ind(level+2) + "<cac:PartyTaxScheme>\n")
		b.WriteString(el(level+3, "cbc:CompanyID", p.VatID))
		b.WriteString(ind(level+3) + "<cac:TaxScheme>\n")
		b.WriteString(el(level+4, "cbc:ID", "VAT"))
		b.WriteString(ind(level+3) + "</cac:TaxScheme>\n")
		b.WriteString(ind(level+2) + "</cac:PartyTaxScheme>\n")
	}
	b.WriteString(ind(level+2) + "<cac:PartyLegalEntity>\n")
	b.WriteString(el(level+3, "cbc:RegistrationName", p.Name))
	b.WriteString(ind(level+2) + "</cac:PartyLegalEntity>\n")
	if seller && strings.TrimSpace(p.Email) != "" {
		b.WriteString(ind(level+2) + "<cac:Contact>\n")
		b.WriteString(el(level+3, "cbc:Name", p.Name))
		b.WriteString(el(level+3, "cbc:ElectronicMail", p.Email))
		b.WriteString(ind(level+2) + "</cac:Contact>\n")
	}
	b.WriteString(ind(level+1) + "</cac:Party>\n")
	b.WriteString(ind(level) + "</" + tag + ">\n")
	return b.String()
}

func ind(level int) string { return strings.Repeat("    ", level) }

func el(level int, tag, value string) string {
	return ind(level) + "<" + tag + ">" + escape(value) + "</" + tag + ">\n"
}

func elAttr(level int, tag, attr, attrValue, value string) string {
	return ind(level) + "<" + tag + " " + attr + `="` + escape(attrValue) + `">` + escape(value) + "</" + tag + ">\n"
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func defaultStr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func unit(u string) string { return defaultStr(u, "C62") }

// money formats integer cents as a decimal with two places.
func money(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := itoa(cents/100) + "." + pad2(cents%100)
	if neg {
		return "-" + s
	}
	return s
}

// qty formats milli-units as a decimal, trimming trailing zeros.
func qty(milli int64) string {
	whole := milli / 1000
	frac := milli % 1000
	if frac == 0 {
		return itoa(whole)
	}
	s := itoa(whole) + "." + trimZeros(pad3(frac))
	return s
}

// rate formats basis points as a percentage, trimming trailing zeros.
func rate(bp int64) string {
	whole := bp / 100
	frac := bp % 100
	if frac == 0 {
		return itoa(whole)
	}
	return itoa(whole) + "." + trimZeros(pad2(frac))
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func pad3(n int64) string {
	s := itoa(n)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func trimZeros(s string) string {
	s = strings.TrimRight(s, "0")
	if s == "" {
		return "0"
	}
	return s
}
