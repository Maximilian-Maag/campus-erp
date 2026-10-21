package sepa

import "testing"

func TestFormatAmount(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0.00"},
		{1, "0.01"},
		{10, "0.10"},
		{100, "1.00"},
		{123456, "1234.56"},
		{-5, "-0.05"},
		{-123456, "-1234.56"},
	}
	for _, c := range cases {
		if got := FormatAmount(c.in); got != c.want {
			t.Errorf("FormatAmount(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidIBAN(t *testing.T) {
	cases := []struct {
		name string
		iban string
		want bool
	}{
		{"canonical DE", "DE89370400440532013000", true},
		{"spaces ignored", "DE89 3704 0044 0532 0130 00", true},
		{"lower case", "de89370400440532013000", true},
		{"bad checksum", "DE89370400440532013001", false},
		{"too short", "DE89", false},
		{"non-letter country code", "1E89370400440532013000", false},
		{"empty", "", false},
		{"invalid characters", "DE89-3704-0044-0532-0130-00", false},

		// Boundary: length. Minimum accepted length is 5 (kills "< 5" -> "<= 5").
		{"minimum length 5", "AB39C", true},
		// Boundary: length. Maximum accepted length is 34 (kills "> 34" -> ">= 34").
		{"maximum length 34", "AB89CCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", true},
		{"one over maximum length 35", "AB89CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", false},

		// Country-code letter boundaries: a country code starting with 'A'
		// kills "c >= 'A'" -> "c > 'A'"; one ending in 'Z' kills
		// "c <= 'Z'" -> "c < 'Z'".
		{"country starts with A", "AD1200012030200359100100", true},
		{"country ends with Z", "AZ21NABZ00000000137010001944", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidIBAN(c.iban); got != c.want {
				t.Errorf("ValidIBAN(%q) = %v, want %v", c.iban, got, c.want)
			}
		})
	}
}
