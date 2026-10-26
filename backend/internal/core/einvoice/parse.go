package einvoice

import (
	"encoding/xml"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Parse errors.
var (
	ErrMalformed = errors.New("einvoice: malformed XRechnung document")
	ErrBadValue  = errors.New("einvoice: a document value could not be parsed")
)

// UBL 2.1 Invoice projection (local names only — the decoder matches regardless
// of the cbc:/cac: namespace prefix).
type ublInvoice struct {
	CustomizationID         string           `xml:"CustomizationID"`
	ProfileID               string           `xml:"ProfileID"`
	ID                      string           `xml:"ID"`
	IssueDate               string           `xml:"IssueDate"`
	InvoiceTypeCode         string           `xml:"InvoiceTypeCode"`
	DocumentCurrencyCode    string           `xml:"DocumentCurrencyCode"`
	BuyerReference          string           `xml:"BuyerReference"`
	AccountingSupplierParty ublPartyWrapper  `xml:"AccountingSupplierParty"`
	AccountingCustomerParty ublPartyWrapper  `xml:"AccountingCustomerParty"`
	PaymentMeans            ublPaymentMeans  `xml:"PaymentMeans"`
	PaymentTerms            ublPaymentTerms  `xml:"PaymentTerms"`
	TaxTotal                ublTaxTotal      `xml:"TaxTotal"`
	LegalMonetaryTotal      ublLegalMonetary `xml:"LegalMonetaryTotal"`
	InvoiceLine             []ublInvoiceLine `xml:"InvoiceLine"`
}

type ublPartyWrapper struct {
	Party ublParty `xml:"Party"`
}

type ublParty struct {
	EndpointID      ublEndpointID      `xml:"EndpointID"`
	PartyName       ublPartyName       `xml:"PartyName"`
	PostalAddress   ublPostalAddress   `xml:"PostalAddress"`
	PartyTaxScheme  ublPartyTaxScheme  `xml:"PartyTaxScheme"`
	PartyLegalEntity ublPartyLegalEntity `xml:"PartyLegalEntity"`
	Contact         ublContact         `xml:"Contact"`
}

type ublEndpointID struct {
	SchemeID string `xml:"schemeID,attr"`
	Value    string `xml:",chardata"`
}

type ublPartyName struct {
	Name string `xml:"Name"`
}

type ublPostalAddress struct {
	StreetName string `xml:"StreetName"`
	CityName   string `xml:"CityName"`
	PostalZone string `xml:"PostalZone"`
	Country    struct {
		IdentificationCode string `xml:"IdentificationCode"`
	} `xml:"Country"`
}

type ublPartyTaxScheme struct {
	CompanyID string `xml:"CompanyID"`
}

type ublPartyLegalEntity struct {
	RegistrationName string `xml:"RegistrationName"`
}

type ublContact struct {
	Name           string `xml:"Name"`
	ElectronicMail string `xml:"ElectronicMail"`
}

type ublPaymentMeans struct {
	PaymentMeansCode      string `xml:"PaymentMeansCode"`
	PayeeFinancialAccount struct {
		ID string `xml:"ID"`
	} `xml:"PayeeFinancialAccount"`
}

type ublPaymentTerms struct {
	Note string `xml:"Note"`
}

type ublTaxTotal struct {
	TaxAmount string `xml:"TaxAmount"`
}

type ublLegalMonetary struct {
	LineExtensionAmount string `xml:"LineExtensionAmount"`
	TaxExclusiveAmount  string `xml:"TaxExclusiveAmount"`
	TaxInclusiveAmount  string `xml:"TaxInclusiveAmount"`
	PayableAmount       string `xml:"PayableAmount"`
}

type ublInvoiceLine struct {
	ID               string `xml:"ID"`
	InvoicedQuantity struct {
		UnitCode string `xml:"unitCode,attr"`
		Value    string `xml:",chardata"`
	} `xml:"InvoicedQuantity"`
	LineExtensionAmount string `xml:"LineExtensionAmount"`
	Item                struct {
		Name                  string `xml:"Name"`
		ClassifiedTaxCategory struct {
			ID      string `xml:"ID"`
			Percent string `xml:"Percent"`
		} `xml:"ClassifiedTaxCategory"`
	} `xml:"Item"`
	Price struct {
		PriceAmount string `xml:"PriceAmount"`
	} `xml:"Price"`
}

// ParseXRechnung parses an XRechnung 3.0 (UBL 2.1) Invoice into our model and
// validates it with the same rules used for outbound invoices.
func ParseXRechnung(doc string) (Invoice, error) {
	var u ublInvoice
	if err := xml.Unmarshal([]byte(doc), &u); err != nil {
		return Invoice{}, ErrMalformed
	}
	if strings.TrimSpace(u.ID) == "" || strings.TrimSpace(u.IssueDate) == "" {
		return Invoice{}, ErrMalformed
	}
	issueDate, err := time.Parse("2006-01-02", strings.TrimSpace(u.IssueDate))
	if err != nil {
		return Invoice{}, ErrBadValue
	}

	inv := Invoice{
		Number:           strings.TrimSpace(u.ID),
		IssueDate:        issueDate,
		TypeCode:         strings.TrimSpace(u.InvoiceTypeCode),
		Currency:         strings.TrimSpace(u.DocumentCurrencyCode),
		BuyerReference:   strings.TrimSpace(u.BuyerReference),
		Seller:           toDomainParty(u.AccountingSupplierParty.Party),
		Buyer:            toDomainParty(u.AccountingCustomerParty.Party),
		PaymentMeansCode: strings.TrimSpace(u.PaymentMeans.PaymentMeansCode),
		PayeeIBAN:        strings.TrimSpace(u.PaymentMeans.PayeeFinancialAccount.ID),
		PaymentTerms:     strings.TrimSpace(u.PaymentTerms.Note),
	}

	for _, l := range u.InvoiceLine {
		qty, err := parseQuantityMilli(l.InvoicedQuantity.Value)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		net, err := parseMoney(l.LineExtensionAmount)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		price, err := parseMoney(l.Price.PriceAmount)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		rateBP, err := parsePercentBP(l.Item.ClassifiedTaxCategory.Percent)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		lineNo, _ := strconv.Atoi(strings.TrimSpace(l.ID))
		inv.Lines = append(inv.Lines, Line{
			LineNo:         lineNo,
			Description:    strings.TrimSpace(l.Item.Name),
			QuantityMilli:  qty,
			UnitCode:       strings.TrimSpace(l.InvoicedQuantity.UnitCode),
			UnitPriceCents: price,
			NetCents:       net,
			VatCategory:    strings.TrimSpace(l.Item.ClassifiedTaxCategory.ID),
			VatRateBP:      rateBP,
		})
	}

	if err := Validate(inv); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

func toDomainParty(p ublParty) Party {
	name := strings.TrimSpace(p.PartyName.Name)
	if name == "" {
		name = strings.TrimSpace(p.PartyLegalEntity.RegistrationName)
	}
	return Party{
		Name:           name,
		Street:         strings.TrimSpace(p.PostalAddress.StreetName),
		City:           strings.TrimSpace(p.PostalAddress.CityName),
		PostalCode:     strings.TrimSpace(p.PostalAddress.PostalZone),
		Country:        strings.TrimSpace(p.PostalAddress.Country.IdentificationCode),
		VatID:          strings.TrimSpace(p.PartyTaxScheme.CompanyID),
		EndpointID:     strings.TrimSpace(p.EndpointID.Value),
		EndpointScheme: strings.TrimSpace(p.EndpointID.SchemeID),
		Email:          strings.TrimSpace(p.Contact.ElectronicMail),
	}
}

// parseMoney parses a decimal amount into cents.
func parseMoney(s string) (int64, error) {
	whole, frac, neg, err := splitDecimal(s)
	if err != nil {
		return 0, err
	}
	n := whole*100 + fracN(frac, 2)
	if neg {
		n = -n
	}
	return n, nil
}

// parseQuantityMilli parses a decimal quantity into milli-units.
func parseQuantityMilli(s string) (int64, error) {
	whole, frac, neg, err := splitDecimal(s)
	if err != nil {
		return 0, err
	}
	n := whole*1000 + fracN(frac, 3)
	if neg {
		n = -n
	}
	return n, nil
}

// parsePercentBP parses a decimal percentage into basis points.
func parsePercentBP(s string) (int64, error) {
	whole, frac, neg, err := splitDecimal(s)
	if err != nil {
		return 0, err
	}
	n := whole*100 + fracN(frac, 2)
	if neg {
		n = -n
	}
	return n, nil
}

// splitDecimal splits "123.45" into whole=123, frac="45". An empty or malformed
// string is an error.
func splitDecimal(s string) (whole int64, frac string, neg bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false, ErrBadValue
	}
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	wholeStr := s
	if i := strings.IndexByte(s, '.'); i >= 0 {
		wholeStr = s[:i]
		frac = s[i+1:]
	}
	if wholeStr == "" {
		wholeStr = "0"
	}
	whole, err = strconv.ParseInt(wholeStr, 10, 64)
	if err != nil {
		return 0, "", false, ErrBadValue
	}
	for _, c := range frac {
		if c < '0' || c > '9' {
			return 0, "", false, ErrBadValue
		}
	}
	return whole, frac, neg, nil
}

// fracN converts the fractional digits to a scaled integer (e.g. "5" with scale
// 2 -> 50, "456" with scale 2 -> 45 truncating, "4" with scale 3 -> 400).
func fracN(frac string, scale int) int64 {
	if len(frac) > scale {
		frac = frac[:scale]
	}
	for len(frac) < scale {
		frac += "0"
	}
	n, _ := strconv.ParseInt(frac, 10, 64)
	return n
}
