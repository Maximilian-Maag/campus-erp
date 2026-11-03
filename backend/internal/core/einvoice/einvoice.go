// Package einvoice models an EN16931 e-invoice and emits XRechnung 3.0 (UBL 2.1).
//
// The model mirrors the EN16931 business groups the reference implementation uses
// (docs/08-reuse-inventory.md §4); the emitted document is the format a German
// public body must accept. Amounts are integer cents, VAT rates are basis points
// and quantities are milli-units (repo conventions, ADR-0001).
package einvoice

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Validation errors.
var (
	ErrNumberRequired       = errors.New("einvoice: an invoice number is required")
	ErrIssueDateRequired    = errors.New("einvoice: an issue date is required")
	ErrCurrency             = errors.New("einvoice: the currency must be a 3-letter ISO code")
	ErrSellerRequired       = errors.New("einvoice: a seller is required")
	ErrBuyerRequired        = errors.New("einvoice: a buyer is required")
	ErrPartyNameRequired    = errors.New("einvoice: a party name is required")
	ErrPartyCityRequired    = errors.New("einvoice: a party city is required")
	ErrPartyPostalCode      = errors.New("einvoice: a party postal code is required")
	ErrPartyCountry         = errors.New("einvoice: a party country (2-letter) is required")
	ErrSellerVATRequired    = errors.New("einvoice: the seller VAT id is required (BR-DE-16)")
	ErrSellerEmailRequired  = errors.New("einvoice: the seller contact e-mail is required (BR-DE-7)")
	ErrBuyerReference       = errors.New("einvoice: a buyer reference is required (BR-DE-15)")
	ErrPaymentMeansRequired = errors.New("einvoice: a payment means code is required")
	ErrPayeeIBANRequired    = errors.New("einvoice: the payee IBAN is required for a credit transfer")
	ErrNoLines              = errors.New("einvoice: at least one invoice line is required")
	ErrLineDescription      = errors.New("einvoice: a line description is required")
	ErrLineQuantity         = errors.New("einvoice: a line quantity must be positive")
	ErrLineAmount           = errors.New("einvoice: a line unit price must not be negative")
	ErrVATCategoryRequired  = errors.New("einvoice: a line VAT category is required")
	ErrVATCategoryUnknown   = errors.New("einvoice: the line VAT category is not a UNTDID 5305 code EN 16931 allows")
	ErrPaymentMeansUnknown  = errors.New("einvoice: the payment means code is not a UNTDID 4461 code EN 16931 allows")
	ErrLineNetMismatch      = errors.New("einvoice: the line net amount does not match quantity x unit price (BR-CO-10)")
	ErrTotalsMismatch       = errors.New("einvoice: the invoice totals are inconsistent (BR-CO-13/14/15/16)")
)

// Code is one entry of a standard code list: the code plus the labels the standard gives it.
type Code struct {
	Code      string
	ShortText string
	LongText  string
}

// vatCategories is UNTDID 5305 restricted to the codes EN 16931 allows a line to carry. The
// standard is the authority here, not tenant data: the catalogue (docs/11) presents the same
// list to operators, and lookup's seed is generated from this one so the two cannot drift.
var vatCategories = []Code{
	{"S", "Regelsteuersatz", "Standard rate"},
	{"Z", "Nullsatz", "Zero rated"},
	{"E", "Steuerfrei", "Exempt from tax"},
	{"AE", "Reverse charge", "VAT reverse charge"},
	{"K", "Innergemeinschaftlich", "Intra-community supply"},
	{"G", "Ausfuhr", "Free export item"},
	{"O", "Nicht im Anwendungsbereich", "Services outside scope of tax"},
	{"L", "Kannaren-Steuer", "IGIC (Canary Islands)"},
	{"M", "IPSI", "IPSI (Ceuta and Melilla)"},
}

// paymentMeansCodes is UNTDID 4461 restricted to the codes EN 16931 allows.
var paymentMeansCodes = []Code{
	{"1", "Nicht definiert", "Instrument not defined"},
	{"10", "Bar", "In cash"},
	{"20", "Scheck", "Cheque"},
	{"30", "Überweisung", "Credit transfer"},
	{"42", "Überweisung auf Sammelkonto", "Payment to bank account"},
	{"48", "Karte", "Bank card"},
	{"49", "SEPA-Lastschrift", "Direct debit"},
	{"57", "Überweisung auf Abrechnungskonto", "Standing order to bank account"},
	{"58", "SEPA-Überweisung", "SEPA credit transfer"},
	{"59", "SEPA-Lastschrift", "SEPA direct debit"},
	{"97", "Verrechnung", "Clearing between partners"},
}

// VATCategories returns the VAT categories a line may carry (UNTDID 5305 as EN 16931 allows).
func VATCategories() []Code { return append([]Code(nil), vatCategories...) }

// PaymentMeansCodes returns the payment means codes an invoice may carry (UNTDID 4461 as
// EN 16931 allows).
func PaymentMeansCodes() []Code { return append([]Code(nil), paymentMeansCodes...) }

// HasCode reports whether code is in list, ignoring surrounding space.
func HasCode(list []Code, code string) bool {
	code = strings.TrimSpace(code)
	for _, c := range list {
		if c.Code == code {
			return true
		}
	}
	return false
}

// Party is a seller or buyer (EN16931 BG-4 / BG-7).
type Party struct {
	Name           string
	Street         string
	City           string
	PostalCode     string
	Country        string
	VatID          string
	EndpointID     string
	EndpointScheme string
	Email          string
}

// Line is one invoice line (EN16931 BG-25).
type Line struct {
	LineNo         int
	Description    string
	QuantityMilli  int64
	UnitCode       string
	UnitPriceCents int64
	NetCents       int64
	VatCategory    string
	VatRateBP      int64
}

// Invoice is an outbound e-invoice.
type Invoice struct {
	ID               int64
	Number           string
	IssueDate        time.Time
	TypeCode         string
	Currency         string
	BuyerReference   string
	Seller           Party
	Buyer            Party
	PaymentMeansCode string
	PayeeIBAN        string
	PaymentTerms     string
	Lines            []Line
	CreatedBy        string
	CreatedAt        time.Time
	// MatchedOrderID is the purchase order this invoice's two-way match bound it to, or 0 while
	// it is unmatched (docs/09, docs/12).
	MatchedOrderID int64
}

// VATGroup is one VAT breakdown line (EN16931 BG-23).
type VATGroup struct {
	Category     string
	RateBP       int64
	TaxableCents int64
	TaxCents     int64
}

// roundHalfUp divides by 10000 with round-half-away-from-zero.
func roundHalfUp(numerator, denominator int64) int64 {
	if numerator >= 0 {
		return (numerator + denominator/2) / denominator
	}
	return -((-numerator + denominator/2) / denominator)
}

// LineNet returns the line's net amount from quantity and unit price.
func LineNet(quantityMilli, unitPriceCents int64) int64 {
	return roundHalfUp(quantityMilli*unitPriceCents, 1000)
}

// VATGroups derives the VAT breakdown from the lines, ordered by category then rate.
func VATGroups(lines []Line) []VATGroup {
	idx := map[string]int{}
	var groups []VATGroup
	for _, l := range lines {
		key := l.VatCategory + "\x00" + itoa(l.VatRateBP)
		i, ok := idx[key]
		if !ok {
			groups = append(groups, VATGroup{Category: l.VatCategory, RateBP: l.VatRateBP})
			i = len(groups) - 1
			idx[key] = i
		}
		groups[i].TaxableCents += l.NetCents
	}
	for i := range groups {
		groups[i].TaxCents = roundHalfUp(groups[i].TaxableCents*groups[i].RateBP, 10000)
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].Category != groups[b].Category {
			return groups[a].Category < groups[b].Category
		}
		return groups[a].RateBP < groups[b].RateBP
	})
	return groups
}

// ComputeTotals returns the net, VAT and gross totals for the invoice.
func ComputeTotals(inv Invoice) (net, vat, gross int64, groups []VATGroup) {
	groups = VATGroups(inv.Lines)
	for _, l := range inv.Lines {
		net += l.NetCents
	}
	for _, g := range groups {
		vat += g.TaxCents
	}
	return net, vat, net + vat, groups
}

// Validate checks the invoice against the rules we enforce (a superset of the
// XRechnung mandatory fields we can satisfy).
func Validate(inv Invoice) error {
	if strings.TrimSpace(inv.Number) == "" {
		return ErrNumberRequired
	}
	if inv.IssueDate.IsZero() {
		return ErrIssueDateRequired
	}
	if len(inv.Currency) != 3 {
		return ErrCurrency
	}
	if err := validateParty(inv.Seller, true); err != nil {
		return err
	}
	if err := validateParty(inv.Buyer, false); err != nil {
		return err
	}
	if strings.TrimSpace(inv.BuyerReference) == "" {
		return ErrBuyerReference
	}
	if strings.TrimSpace(inv.PaymentMeansCode) == "" {
		return ErrPaymentMeansRequired
	}
	if !HasCode(paymentMeansCodes, inv.PaymentMeansCode) {
		return ErrPaymentMeansUnknown
	}
	if strings.TrimSpace(inv.PayeeIBAN) == "" {
		return ErrPayeeIBANRequired
	}
	if len(inv.Lines) == 0 {
		return ErrNoLines
	}
	for _, l := range inv.Lines {
		if strings.TrimSpace(l.Description) == "" {
			return ErrLineDescription
		}
		if l.QuantityMilli <= 0 {
			return ErrLineQuantity
		}
		if l.UnitPriceCents < 0 {
			return ErrLineAmount
		}
		if strings.TrimSpace(l.VatCategory) == "" {
			return ErrVATCategoryRequired
		}
		if !HasCode(vatCategories, l.VatCategory) {
			return ErrVATCategoryUnknown
		}
		if l.NetCents != LineNet(l.QuantityMilli, l.UnitPriceCents) {
			return ErrLineNetMismatch
		}
	}
	return nil
}

func validateParty(p Party, seller bool) error {
	if strings.TrimSpace(p.Name) == "" {
		if seller {
			return ErrSellerRequired
		}
		return ErrBuyerRequired
	}
	if strings.TrimSpace(p.City) == "" {
		return ErrPartyCityRequired
	}
	if strings.TrimSpace(p.PostalCode) == "" {
		return ErrPartyPostalCode
	}
	if len(p.Country) != 2 {
		return ErrPartyCountry
	}
	if seller {
		if strings.TrimSpace(p.VatID) == "" {
			return ErrSellerVATRequired
		}
		if strings.TrimSpace(p.Email) == "" {
			return ErrSellerEmailRequired
		}
	}
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
