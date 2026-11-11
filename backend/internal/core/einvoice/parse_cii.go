package einvoice

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

// Reading a CII document (UN/CEFACT CrossIndustryInvoice, the syntax ZUGFeRD/Factur-X uses).
//
// This is the counterpart of ParseXRechnung: a supplier that sends CII rather than UBL is sending
// the same invoice in another syntax, so both readers produce the same model and the same
// validations apply. The element paths below mirror the writer in cii.go element for element — what
// this package emits must read back into the model it was built from, which is what the round-trip
// test pins.
//
// Only the syntax this package writes is read: the EN 16931 profile. A document that carries
// something else (a different profile, a PDF/A-3 container's embedded file) is refused rather than
// silently half-read.

// ciiDocument is the root: rsm:CrossIndustryInvoice.
type ciiDocument struct {
	XMLName     xml.Name             `xml:"CrossIndustryInvoice"`
	Document    ciiExchangedDocument `xml:"ExchangedDocument"`
	Transaction ciiTradeTransaction  `xml:"SupplyChainTradeTransaction"`
}

type ciiExchangedDocument struct {
	ID            string `xml:"ID"`
	TypeCode      string `xml:"TypeCode"`
	IssueDateTime struct {
		DateTimeString struct {
			Format string `xml:"format,attr"`
			Value  string `xml:",chardata"`
		} `xml:"DateTimeString"`
	} `xml:"IssueDateTime"`
}

type ciiTradeTransaction struct {
	Lines     []ciiLine `xml:"IncludedSupplyChainTradeLineItem"`
	Agreement struct {
		BuyerReference string   `xml:"BuyerReference"`
		Seller         ciiParty `xml:"SellerTradeParty"`
		Buyer          ciiParty `xml:"BuyerTradeParty"`
	} `xml:"ApplicableHeaderTradeAgreement"`
	Settlement struct {
		Currency     string `xml:"InvoiceCurrencyCode"`
		PaymentMeans struct {
			TypeCode string `xml:"TypeCode"`
			Account  struct {
				IBAN string `xml:"IBANID"`
			} `xml:"PayeePartyCreditorFinancialAccount"`
		} `xml:"SpecifiedTradeSettlementPaymentMeans"`
		PaymentTerms struct {
			Description string `xml:"Description"`
		} `xml:"SpecifiedTradePaymentTerms"`
	} `xml:"ApplicableHeaderTradeSettlement"`
}

type ciiLine struct {
	LineID  string `xml:"AssociatedDocumentLineDocument>LineID"`
	Product struct {
		Name string `xml:"Name"`
	} `xml:"SpecifiedTradeProduct"`
	Agreement struct {
		NetPrice struct {
			ChargeAmount string `xml:"ChargeAmount"`
		} `xml:"NetPriceProductTradePrice"`
	} `xml:"SpecifiedLineTradeAgreement"`
	Delivery struct {
		BilledQuantity struct {
			UnitCode string `xml:"unitCode,attr"`
			Value    string `xml:",chardata"`
		} `xml:"BilledQuantity"`
	} `xml:"SpecifiedLineTradeDelivery"`
	Settlement struct {
		Tax struct {
			CategoryCode string `xml:"CategoryCode"`
			Rate         string `xml:"RateApplicablePercent"`
		} `xml:"ApplicableTradeTax"`
		Summation struct {
			LineTotalAmount string `xml:"LineTotalAmount"`
		} `xml:"SpecifiedTradeSettlementLineMonetarySummation"`
	} `xml:"SpecifiedLineTradeSettlement"`
}

type ciiParty struct {
	Name    string `xml:"Name"`
	Address struct {
		Street     string `xml:"LineOne"`
		PostalCode string `xml:"PostcodeCode"`
		City       string `xml:"CityName"`
		Country    string `xml:"CountryID"`
	} `xml:"PostalTradeAddress"`
	TaxRegistration struct {
		ID ciiSchemeID `xml:"ID"`
	} `xml:"SpecifiedTaxRegistration"`
	URI struct {
		ID ciiSchemeID `xml:"URIID"`
	} `xml:"URIUniversalCommunication"`
	Contact struct {
		PersonName string `xml:"PersonName"`
		Email      struct {
			URIID string `xml:"URIID"`
		} `xml:"EmailURIUniversalCommunication"`
	} `xml:"DefinedTradeContact"`
}

// ciiSchemeID is a value that carries a scheme in an attribute, e.g. the tax registration's `VA`.
type ciiSchemeID struct {
	Scheme string `xml:"schemeID,attr"`
	Value  string `xml:",chardata"`
}

// ParseCII parses a UN/CEFACT CrossIndustryInvoice into our model and validates it with the same
// rules an outbound invoice is held to.
func ParseCII(doc string) (Invoice, error) {
	var d ciiDocument
	if err := xml.Unmarshal([]byte(doc), &d); err != nil {
		return Invoice{}, ErrMalformed
	}
	number := strings.TrimSpace(d.Document.ID)
	if number == "" {
		return Invoice{}, ErrMalformed
	}
	issueDate, err := parseCIIssueDate(d.Document.IssueDateTime.DateTimeString.Format,
		d.Document.IssueDateTime.DateTimeString.Value)
	if err != nil {
		return Invoice{}, err
	}

	inv := Invoice{
		Number:    number,
		IssueDate: issueDate,
		TypeCode:  strings.TrimSpace(d.Document.TypeCode),
		Currency:  strings.TrimSpace(d.Transaction.Settlement.Currency),
		// BT-10: the buyer reference the order is found by.
		BuyerReference:   strings.TrimSpace(d.Transaction.Agreement.BuyerReference),
		Seller:           toDomainCIIParty(d.Transaction.Agreement.Seller),
		Buyer:            toDomainCIIParty(d.Transaction.Agreement.Buyer),
		PaymentMeansCode: strings.TrimSpace(d.Transaction.Settlement.PaymentMeans.TypeCode),
		PayeeIBAN:        strings.TrimSpace(d.Transaction.Settlement.PaymentMeans.Account.IBAN),
		PaymentTerms:     strings.TrimSpace(d.Transaction.Settlement.PaymentTerms.Description),
	}

	for _, l := range d.Transaction.Lines {
		qty, err := parseQuantityMilli(l.Delivery.BilledQuantity.Value)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		net, err := parseMoney(l.Settlement.Summation.LineTotalAmount)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		price, err := parseMoney(l.Agreement.NetPrice.ChargeAmount)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		rateBP, err := parsePercentBP(l.Settlement.Tax.Rate)
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		lineNo, err := strconv.Atoi(strings.TrimSpace(l.LineID))
		if err != nil {
			return Invoice{}, ErrBadValue
		}
		inv.Lines = append(inv.Lines, Line{
			LineNo:         lineNo,
			Description:    strings.TrimSpace(l.Product.Name),
			QuantityMilli:  qty,
			UnitCode:       strings.TrimSpace(l.Delivery.BilledQuantity.UnitCode),
			UnitPriceCents: price,
			NetCents:       net,
			VatCategory:    strings.TrimSpace(l.Settlement.Tax.CategoryCode),
			VatRateBP:      rateBP,
		})
	}

	if err := Validate(inv); err != nil {
		return Invoice{}, err
	}
	return inv, nil
}

// parseCIIssueDate reads the CII date. Format 102 is YYYYMMDD, which is what EN 16931 documents
// carry; anything else is refused rather than guessed at.
func parseCIIssueDate(format, value string) (time.Time, error) {
	if strings.TrimSpace(format) != "102" {
		return time.Time{}, ErrBadValue
	}
	date, err := time.Parse("20060102", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, ErrBadValue
	}
	return date, nil
}

func toDomainCIIParty(p ciiParty) Party {
	party := Party{
		Name:       strings.TrimSpace(p.Name),
		Street:     strings.TrimSpace(p.Address.Street),
		City:       strings.TrimSpace(p.Address.City),
		PostalCode: strings.TrimSpace(p.Address.PostalCode),
		Country:    strings.TrimSpace(p.Address.Country),
		// The CII counterpart of the UBL endpoint: the electronic address and the scheme it is in.
		EndpointID:     strings.TrimSpace(p.URI.ID.Value),
		EndpointScheme: strings.TrimSpace(p.URI.ID.Scheme),
		Email:          strings.TrimSpace(p.Contact.Email.URIID),
	}
	// Only the VAT registration is a tax id we carry; another scheme would be a different identifier
	// and must not be read as the VAT number.
	if scheme := strings.TrimSpace(p.TaxRegistration.ID.Scheme); scheme == "VA" || scheme == "" {
		party.VatID = strings.TrimSpace(p.TaxRegistration.ID.Value)
	}
	return party
}
