package kasse

import (
	"errors"
	"strings"
)

var (
	// ErrVATCodeRequired is returned when a VAT type has no code.
	ErrVATCodeRequired = errors.New("kasse: a VAT code is required")
	// ErrVATCodeTooLong is returned for a VAT code longer than 8 characters.
	ErrVATCodeTooLong = errors.New("kasse: a VAT code must be at most 8 characters")
	// ErrVATRateOutOfRange is returned for a rate outside 0..100 %.
	ErrVATRateOutOfRange = errors.New("kasse: a VAT rate must be between 0 and 100 percent")
	// ErrVATKindUnknown is returned for a kind other than output/input.
	ErrVATKindUnknown = errors.New("kasse: VAT kind must be 'output' or 'input'")
	// ErrAmountNegative is returned when a tax amount is negative.
	ErrAmountNegative = errors.New("kasse: the amount must not be negative")
	// ErrVATCodeExists is returned when the VAT code is already in use.
	ErrVATCodeExists = errors.New("kasse: a VAT type with that code already exists")
	// ErrTaxOfficeExists is returned when the (year, number) pair is taken.
	ErrTaxOfficeExists = errors.New("kasse: a tax office with that year and number already exists")
	// ErrFoundationExists is returned when the (year, number) pair is taken.
	ErrFoundationExists = errors.New("kasse: a foundation with that year and number already exists")
)

// VATKind distinguishes output tax (Umsatzsteuer) from input tax (Vorsteuer).
type VATKind string

const (
	VATOutput VATKind = "output"
	VATInput  VATKind = "input"
)

// maxVATCodeLen matches the legacy varchar(8) column.
const maxVATCodeLen = 8

// VATType is a VAT category (legacy kbs.mwst_art).
type VATType struct {
	ID              int64
	Code            string
	Description     string
	RateBasisPoints int64
	Kind            VATKind
}

// NewVATType validates and builds a VAT type.
func NewVATType(code, description string, rateBasisPoints int64, kind VATKind) (VATType, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return VATType{}, ErrVATCodeRequired
	}
	if len(code) > maxVATCodeLen {
		return VATType{}, ErrVATCodeTooLong
	}
	if rateBasisPoints < 0 || rateBasisPoints > 10000 {
		return VATType{}, ErrVATRateOutOfRange
	}
	if kind != VATOutput && kind != VATInput {
		return VATType{}, ErrVATKindUnknown
	}
	return VATType{
		Code:            code,
		Description:     strings.TrimSpace(description),
		RateBasisPoints: rateBasisPoints,
		Kind:            kind,
	}, nil
}

// TaxOn is the tax contained in, or added to, a net amount.
func (v VATType) TaxOn(netCents int64) (int64, error) {
	if netCents < 0 {
		return 0, ErrAmountNegative
	}
	return roundDiv(netCents*v.RateBasisPoints, 10000), nil
}

// GrossFromNet returns the gross amount for a net amount.
func (v VATType) GrossFromNet(netCents int64) (int64, error) {
	tax, err := v.TaxOn(netCents)
	if err != nil {
		return 0, err
	}
	return netCents + tax, nil
}

// NetFromGross extracts the net amount contained in a gross amount.
func (v VATType) NetFromGross(grossCents int64) (int64, error) {
	if grossCents < 0 {
		return 0, ErrAmountNegative
	}
	return roundDiv(grossCents*10000, 10000+v.RateBasisPoints), nil
}

// TaxFromGross is the tax contained in a gross amount. It is defined as the
// remainder, so net + tax always equals the gross exactly.
func (v VATType) TaxFromGross(grossCents int64) (int64, error) {
	net, err := v.NetFromGross(grossCents)
	if err != nil {
		return 0, err
	}
	return grossCents - net, nil
}

// TaxOffice is a tax-office master record (legacy kbs.finanzamt).
type TaxOffice struct {
	ID            int64
	Year          int
	Number        string
	Name          string
	Street        string
	PostalCode    string
	City          string
	AccountHolder string
	IBAN          string
	BIC           string
}

// NewTaxOffice validates and builds a tax-office record.
func NewTaxOffice(year int, number, name, street, postalCode, city, accountHolder, iban, bic string) (TaxOffice, error) {
	if year < 1900 || year > 2999 {
		return TaxOffice{}, ErrBadYear
	}
	number = strings.TrimSpace(number)
	if number == "" {
		return TaxOffice{}, ErrNumberRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return TaxOffice{}, ErrNameRequired
	}
	iban = NormalizeIBAN(iban)
	if iban != "" && !ValidIBAN(iban) {
		return TaxOffice{}, ErrIBANInvalid
	}
	return TaxOffice{
		Year: year, Number: number, Name: name,
		Street: strings.TrimSpace(street), PostalCode: strings.TrimSpace(postalCode),
		City: strings.TrimSpace(city), AccountHolder: strings.TrimSpace(accountHolder),
		IBAN: iban, BIC: strings.ToUpper(strings.TrimSpace(bic)),
	}, nil
}

// Foundation is a foundation/fund reference (legacy kbs.stift).
type Foundation struct {
	ID          int64
	Year        int
	Number      string
	Description string
	CarryOver   bool
}

// NewFoundation validates and builds a foundation reference.
func NewFoundation(year int, number, description string, carryOver bool) (Foundation, error) {
	if year < 1900 || year > 2999 {
		return Foundation{}, ErrBadYear
	}
	number = strings.TrimSpace(number)
	if number == "" {
		return Foundation{}, ErrNumberRequired
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return Foundation{}, ErrNameRequired
	}
	return Foundation{Year: year, Number: number, Description: description, CarryOver: carryOver}, nil
}
