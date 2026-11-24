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

// Retiring is soft state: the row stays, the flag goes, and a second revocation is refused rather
// than repeated as if it had taken effect.
func TestMandateRetire(t *testing.T) {
	active := Mandate{Reference: "MAN-1", Active: true, SequenceType: SequenceRecurring}
	retired, err := active.Retire()
	if err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if retired.Active {
		t.Fatalf("retired = %+v, want it inactive", retired)
	}
	if !active.Active {
		t.Fatal("Retire changed the mandate it was called on instead of returning a retired copy")
	}
	if retired.Reference != active.Reference || retired.SequenceType != active.SequenceType {
		t.Fatalf("retired = %+v, want everything but the flag", retired)
	}
	if _, err := retired.Retire(); !errors.Is(err, ErrMandateRetired) {
		t.Fatalf("retiring a retired mandate = %v, want ErrMandateRetired", err)
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

// The first collection under a FRST mandate is the only one that may be collected as FRST, so the
// mandate moves to RCUR; every other type is already at or past its last allowed collection and
// stays as it is.
func TestMandateAdvanceAfterCollection(t *testing.T) {
	cases := []struct {
		from  MandateSequence
		want  MandateSequence
		moved bool
	}{
		{SequenceFirst, SequenceRecurring, true},
		{SequenceRecurring, SequenceRecurring, false},
		{SequenceOneOff, SequenceOneOff, false},
		{SequenceFinal, SequenceFinal, false},
	}
	for _, c := range cases {
		m := Mandate{Reference: "MAN-1", SequenceType: c.from}
		got, moved := m.AdvanceAfterCollection()
		if got.SequenceType != c.want || moved != c.moved {
			t.Errorf("AdvanceAfterCollection(%q) = %q, %t; want %q, %t", c.from, got.SequenceType, moved, c.want, c.moved)
		}
		if m.SequenceType != c.from {
			t.Errorf("AdvanceAfterCollection(%q) changed the receiver", c.from)
		}
	}
}

// Marking a mandate final is the operator's request for the last collection: a recurring mandate
// moves to FNAL, and every other type is refused rather than silently accepted.
func TestMandateMarkFinal(t *testing.T) {
	recurring := Mandate{Reference: "MAN-1", SequenceType: SequenceRecurring}
	final, err := recurring.MarkFinal()
	if err != nil {
		t.Fatalf("MarkFinal: %v", err)
	}
	if final.SequenceType != SequenceFinal {
		t.Fatalf("MarkFinal = %q, want FNAL", final.SequenceType)
	}
	if recurring.SequenceType != SequenceRecurring {
		t.Fatal("MarkFinal changed the receiver")
	}

	cases := []struct {
		from MandateSequence
		want error
	}{
		{SequenceFirst, ErrMandateNotYetCollected},
		{SequenceOneOff, ErrMandateOneOff},
		{SequenceFinal, ErrMandateAlreadyFinal},
		{"X", ErrSequenceTypeUnknown},
	}
	for _, c := range cases {
		if _, err := (Mandate{Reference: "MAN-1", SequenceType: c.from}).MarkFinal(); !errors.Is(err, c.want) {
			t.Errorf("MarkFinal(%q) = %v, want %v", c.from, err, c.want)
		}
	}
}
