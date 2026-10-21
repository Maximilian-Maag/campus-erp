// Package sepa holds SEPA helpers (ISO 20022) used by the finance and kasse
// modules. Formats are legally binding (ADR: interface parity, IR-2).
package sepa

import (
	"fmt"
	"strings"
)

// FormatAmount renders a cent amount in pain.001 form: dot decimal, no grouping,
// always two fraction digits. FormatAmount(123456) == "1234.56".
func FormatAmount(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		return "-" + s
	}
	return s
}

// ValidIBAN reports whether iban is structurally valid and passes the ISO 7064
// mod-97 checksum. Spaces are ignored; case is normalised.
func ValidIBAN(iban string) bool {
	s := strings.ToUpper(strings.ReplaceAll(iban, " ", ""))
	if len(s) < 5 || len(s) > 34 {
		return false
	}
	if !isLetter(rune(s[0])) || !isLetter(rune(s[1])) {
		return false
	}
	rearranged := s[4:] + s[:4]

	rem := 0
	for _, c := range rearranged {
		switch {
		case isDigit(c):
			rem = (rem*10 + int(c-'0')) % 97
		case isLetter(c):
			v := int(c-'A') + 10
			rem = (rem*10 + v/10) % 97
			rem = (rem*10 + v%10) % 97
		default:
			return false
		}
	}
	return rem == 1
}

func isDigit(c rune) bool { return c >= '0' && c <= '9' }

func isLetter(c rune) bool { return c >= 'A' && c <= 'Z' }
