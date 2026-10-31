package kasse

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidSequenceType(t *testing.T) {
	for _, s := range []MandateSequence{SequenceFirst, SequenceRecurring, SequenceOneOff, SequenceFinal} {
		if !ValidSequenceType(s) {
			t.Errorf("ValidSequenceType(%q) = false", s)
		}
	}
	for _, s := range []MandateSequence{"", "rcur", "FRST ", "X"} {
		if ValidSequenceType(s) {
			t.Errorf("ValidSequenceType(%q) = true", s)
		}
	}
}

func TestNewMandate(t *testing.T) {
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	m, err := NewMandate("MAN-1", "Max Mustermann", "DE89370400440532013000", "cobadeffxxx", sig, SequenceRecurring)
	if err != nil {
		t.Fatalf("NewMandate: %v", err)
	}
	if m.Reference != "MAN-1" || m.SequenceType != SequenceRecurring || !m.Active {
		t.Fatalf("mandate = %+v", m)
	}
	if m.DebtorBIC != "COBADEFFXXX" {
		t.Fatalf("BIC not upper-cased: %q", m.DebtorBIC)
	}
	// A mandate without a BIC is valid (BIC is optional in SEPA).
	if _, err := NewMandate("MAN-2", "Erika", "DE89370400440532013000", "", sig, SequenceFirst); err != nil {
		t.Fatalf("BIC is optional: %v", err)
	}
}

func TestNewMandateValidation(t *testing.T) {
	sig := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		mut  func() (string, string, string, string, time.Time, MandateSequence)
		want error
	}{
		{"blank reference", func() (string, string, string, string, time.Time, MandateSequence) {
			return "  ", "Max", "DE89370400440532013000", "", sig, SequenceRecurring
		}, ErrMandateReferenceRequired},
		{"reference too long", func() (string, string, string, string, time.Time, MandateSequence) {
			return strings.Repeat("x", 36), "Max", "DE89370400440532013000", "", sig, SequenceRecurring
		}, ErrMandateReferenceTooLong},
		{"blank name", func() (string, string, string, string, time.Time, MandateSequence) {
			return "MAN", "", "DE89370400440532013000", "", sig, SequenceRecurring
		}, ErrNameRequired},
		{"bad iban", func() (string, string, string, string, time.Time, MandateSequence) {
			return "MAN", "Max", "DE00", "", sig, SequenceRecurring
		}, ErrIBANInvalid},
		{"bad bic", func() (string, string, string, string, time.Time, MandateSequence) {
			return "MAN", "Max", "DE89370400440532013000", "nope", sig, SequenceRecurring
		}, ErrDebtorBICInvalid},
		{"zero signature date", func() (string, string, string, string, time.Time, MandateSequence) {
			return "MAN", "Max", "DE89370400440532013000", "", time.Time{}, SequenceRecurring
		}, ErrSignatureDateRequired},
		{"bad sequence type", func() (string, string, string, string, time.Time, MandateSequence) {
			return "MAN", "Max", "DE89370400440532013000", "", sig, "X"
		}, ErrSequenceTypeUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref, name, iban, bic, sd, seq := c.mut()
			if _, err := NewMandate(ref, name, iban, bic, sd, seq); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
