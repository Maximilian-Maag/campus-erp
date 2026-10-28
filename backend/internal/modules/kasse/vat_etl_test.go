package kasse

import "testing"

func TestPercentToBasisPoints(t *testing.T) {
	cases := map[string]int64{
		"19.00":  1900,
		"19":     1900,
		"19.5":   1950,
		"7":      700,
		"0.00":   0,
		"100":    10000,
		"19.999": 1999, // truncated past the second decimal
		"+5.25":  525,
		"-5.00":  -500,
		" 7.00 ": 700,
	}
	for in, want := range cases {
		got, err := percentToBasisPoints(in)
		if err != nil || got != want {
			t.Errorf("percentToBasisPoints(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", ".5", "1e2", "1.2.3", "x.y"} {
		if _, err := percentToBasisPoints(bad); err == nil {
			t.Errorf("percentToBasisPoints(%q) should fail", bad)
		}
	}
}

func TestVATKindFromLegacy(t *testing.T) {
	cases := map[string]VATKind{
		"U": VATOutput, "u": VATOutput,
		"V": VATInput, "v": VATInput,
		"":  VATOutput, // unset defaults to output (documented inference)
		"X": VATOutput, "  ": VATOutput,
	}
	for in, want := range cases {
		if got := vatKindFromLegacy(in); got != want {
			t.Errorf("vatKindFromLegacy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBoolFromLegacyFlag(t *testing.T) {
	for _, in := range []string{"J", "j", "Y", "1", "T", " t "} {
		if !boolFromLegacyFlag(in) {
			t.Errorf("boolFromLegacyFlag(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "N", "0", "x", "  "} {
		if boolFromLegacyFlag(in) {
			t.Errorf("boolFromLegacyFlag(%q) = true, want false", in)
		}
	}
}
