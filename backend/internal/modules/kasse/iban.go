package kasse

import (
	"errors"
	"strings"
)

var (
	// ErrIBANInvalid is returned for an IBAN that fails its check digits.
	ErrIBANInvalid = errors.New("kasse: invalid IBAN")
	// ErrNumberRequired is returned when a business number is missing.
	ErrNumberRequired = errors.New("kasse: a number is required")
	// ErrNameRequired is returned when a name or description is missing.
	ErrNameRequired = errors.New("kasse: a name or description is required")
)

// NormalizeIBAN removes separators and upper-cases an IBAN.
func NormalizeIBAN(iban string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(iban) {
		if r == ' ' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ValidIBAN checks the structure and the ISO 7064 mod-97 check digits.
func ValidIBAN(iban string) bool {
	iban = NormalizeIBAN(iban)
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}
	if !isUpperLetter(rune(iban[0])) || !isUpperLetter(rune(iban[1])) {
		return false
	}
	if !isDigit(rune(iban[2])) || !isDigit(rune(iban[3])) {
		return false
	}
	// Everything after the check digits must be alphanumeric. Iterating the slice
	// directly avoids index arithmetic.
	for _, r := range iban[4:] {
		if !isUpperLetter(r) && !isDigit(r) {
			return false
		}
	}
	// Move the first four characters to the end, then evaluate mod 97 over the
	// digits, where A=10 .. Z=35.
	rearranged := iban[4:] + iban[:4]
	remainder := 0
	for _, r := range rearranged {
		if isDigit(r) {
			remainder = (remainder*10 + int(r-'0')) % 97
			continue
		}
		value := int(r-'A') + 10
		remainder = (remainder*100 + value) % 97
	}
	return remainder == 1
}

func isUpperLetter(r rune) bool { return r >= 'A' && r <= 'Z' }

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
