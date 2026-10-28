package kasse

import (
	"context"
	"errors"
	"testing"
)

// Verified check digits (ISO 7064 mod-97).
const (
	validIBANDE = "DE02120300000000202051"
	validIBANGB = "GB82WEST12345698765432"
)

func TestIBANCharacterHelpers(t *testing.T) {
	for _, r := range []rune{'0', '5', '9'} {
		if !isDigit(r) {
			t.Errorf("isDigit(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{'/', ':', 'A', 'a', ' '} {
		if isDigit(r) {
			t.Errorf("isDigit(%q) = true, want false", r)
		}
	}
	for _, r := range []rune{'A', 'M', 'Z'} {
		if !isUpperLetter(r) {
			t.Errorf("isUpperLetter(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{'@', '[', 'a', 'z', '0'} {
		if isUpperLetter(r) {
			t.Errorf("isUpperLetter(%q) = true, want false", r)
		}
	}
}

func TestValidIBAN(t *testing.T) {
	cases := []struct {
		iban string
		want bool
	}{
		{validIBANDE, true},
		{validIBANGB, true},
		{"FR7630006000011234567890189", true},
		{"DE02 1203 0000 0000 2020 51", true}, // separators are ignored
		{"de02120300000000202051", true},      // lower case is normalised
		{"DE02-1203-0000-0000-2020-51", true},
		{"DE02120300000000202052", false}, // wrong check digits
		{"DE01120300000000202051", false}, // check digits yield remainder 0
		{"DE03500105170123456789", false},
		{"DE02120300000000202", false},    // too short
		{"1E02120300000000202051", false}, // must start with two letters
		{"DE0A120300000000202051", false}, // positions 3-4 must be digits
		{"DE0212030000000020205!", false}, // illegal character
		// Length boundaries: 15 and 34 are valid, 14 and 35 are not.
		{"NO9386011117947", true},
		{"DE75111111111111111111111111111111", true},
		{"NO938601111794", false},
		{"DE751111111111111111111111111111111", false},
		// Positions 3 and 4 must both be digits.
		{"DEA0120300000000202051", false},
		{"DE0A120300000000202051", false},
	}
	for _, c := range cases {
		if got := ValidIBAN(c.iban); got != c.want {
			t.Errorf("ValidIBAN(%q) = %v, want %v", c.iban, got, c.want)
		}
	}
}

func TestNormalizeIBAN(t *testing.T) {
	if got := NormalizeIBAN(" de02 1203-0000 0000 2020 51 "); got != validIBANDE {
		t.Fatalf("NormalizeIBAN = %q, want %q", got, validIBANDE)
	}
}

func TestNewVATType(t *testing.T) {
	v, err := NewVATType("  19  ", "  Regelsteuersatz  ", 1900, VATOutput)
	if err != nil {
		t.Fatalf("NewVATType: %v", err)
	}
	if v.Code != "19" || v.Description != "Regelsteuersatz" || v.RateBasisPoints != 1900 || v.Kind != VATOutput {
		t.Fatalf("vat = %+v", v)
	}

	cases := []struct {
		name string
		want error
		code string
		rate int64
		kind VATKind
	}{
		{"blank code", ErrVATCodeRequired, "  ", 1900, VATOutput},
		{"code too long", ErrVATCodeTooLong, "123456789", 1900, VATOutput},
		{"negative rate", ErrVATRateOutOfRange, "19", -1, VATOutput},
		{"rate above 100 percent", ErrVATRateOutOfRange, "19", 10001, VATOutput},
		{"unknown kind", ErrVATKindUnknown, "19", 1900, "sideways"},
		{"empty kind", ErrVATKindUnknown, "19", 1900, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewVATType(c.code, "", c.rate, c.kind); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}

	// The boundaries are inclusive.
	if _, err := NewVATType("0", "", 0, VATInput); err != nil {
		t.Fatalf("a zero rate must be accepted: %v", err)
	}
	if _, err := NewVATType("100", "", 10000, VATInput); err != nil {
		t.Fatalf("a 100 percent rate must be accepted: %v", err)
	}
	if _, err := NewVATType("12345678", "", 0, VATInput); err != nil {
		t.Fatalf("an 8-character code must be accepted: %v", err)
	}
}

func TestVATAritmetic(t *testing.T) {
	vat := VATType{Code: "19", RateBasisPoints: 1900, Kind: VATOutput}

	tax, err := vat.TaxOn(10000)
	if err != nil {
		t.Fatalf("TaxOn: %v", err)
	}
	if tax != 1900 {
		t.Fatalf("tax = %d, want 1900", tax)
	}

	gross, err := vat.GrossFromNet(10000)
	if err != nil {
		t.Fatalf("GrossFromNet: %v", err)
	}
	if gross != 11900 {
		t.Fatalf("gross = %d, want 11900", gross)
	}

	net, err := vat.NetFromGross(11900)
	if err != nil {
		t.Fatalf("NetFromGross: %v", err)
	}
	if net != 10000 {
		t.Fatalf("net = %d, want 10000", net)
	}

	// Half-up rounding on the tax.
	if got, _ := vat.TaxOn(12550); got != 2385 { // 2384.5 -> 2385
		t.Fatalf("TaxOn(12550) = %d, want 2385", got)
	}
	// Zero is a legal amount: only negative values are refused.
	if got, err := vat.TaxOn(0); err != nil || got != 0 {
		t.Fatalf("TaxOn(0) = %d, %v; want 0 with no error", got, err)
	}

	if _, err := vat.TaxOn(-1); !errors.Is(err, ErrAmountNegative) {
		t.Fatalf("negative net: want ErrAmountNegative, got %v", err)
	}
	if _, err := vat.NetFromGross(-1); !errors.Is(err, ErrAmountNegative) {
		t.Fatalf("negative gross: want ErrAmountNegative, got %v", err)
	}
	if _, err := vat.GrossFromNet(-1); !errors.Is(err, ErrAmountNegative) {
		t.Fatalf("negative net: want ErrAmountNegative, got %v", err)
	}
	if _, err := vat.TaxFromGross(-1); !errors.Is(err, ErrAmountNegative) {
		t.Fatalf("negative gross: want ErrAmountNegative, got %v", err)
	}
}

// net + tax must equal the gross exactly, for every amount and rate.
func TestVATTaxFromGrossIsTheRemainder(t *testing.T) {
	rates := []int64{0, 700, 1900, 10000}
	for _, rate := range rates {
		vat := VATType{Code: "x", RateBasisPoints: rate}
		for gross := int64(0); gross <= 2000; gross += 7 {
			net, err := vat.NetFromGross(gross)
			if err != nil {
				t.Fatalf("NetFromGross(%d): %v", gross, err)
			}
			tax, err := vat.TaxFromGross(gross)
			if err != nil {
				t.Fatalf("TaxFromGross(%d): %v", gross, err)
			}
			if net+tax != gross {
				t.Fatalf("rate %d gross %d: net %d + tax %d != gross", rate, gross, net, tax)
			}
			if tax < 0 {
				t.Fatalf("rate %d gross %d: negative tax %d", rate, gross, tax)
			}
		}
	}
}

func TestNewTaxOffice(t *testing.T) {
	office, err := NewTaxOffice(2026, " 1234 ", " Finanzamt Musterstadt ", " Weg 1 ", "12345",
		" Musterstadt ", "Land Muster", validIBANDE, "byladem1001")
	if err != nil {
		t.Fatalf("NewTaxOffice: %v", err)
	}
	if office.Number != "1234" || office.Name != "Finanzamt Musterstadt" || office.BIC != "BYLADEM1001" {
		t.Fatalf("office = %+v", office)
	}
	if office.IBAN != validIBANDE {
		t.Fatalf("iban = %q", office.IBAN)
	}

	// A missing IBAN is allowed: not every tax office is reachable by SEPA.
	if _, err := NewTaxOffice(2026, "1", "Amt", "", "", "", "", "", ""); err != nil {
		t.Fatalf("an empty IBAN must be accepted: %v", err)
	}

	// The year boundaries are inclusive.
	for _, year := range []int{1900, 2999} {
		if _, err := NewTaxOffice(year, "1", "Amt", "", "", "", "", "", ""); err != nil {
			t.Fatalf("year %d must be accepted: %v", year, err)
		}
	}

	cases := []struct {
		name string
		want error
		fn   func() (TaxOffice, error)
	}{
		{"bad year", ErrBadYear, func() (TaxOffice, error) { return NewTaxOffice(1800, "1", "Amt", "", "", "", "", "", "") }},
		{"blank number", ErrNumberRequired, func() (TaxOffice, error) { return NewTaxOffice(2026, "  ", "Amt", "", "", "", "", "", "") }},
		{"blank name", ErrNameRequired, func() (TaxOffice, error) { return NewTaxOffice(2026, "1", "  ", "", "", "", "", "", "") }},
		{"bad iban", ErrIBANInvalid, func() (TaxOffice, error) {
			return NewTaxOffice(2026, "1", "Amt", "", "", "", "", "DE02120300000000202052", "")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.fn(); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestNewFoundation(t *testing.T) {
	f, err := NewFoundation(2026, " 07 ", " Deutschlandstipendium ", true)
	if err != nil {
		t.Fatalf("NewFoundation: %v", err)
	}
	if f.Number != "07" || f.Description != "Deutschlandstipendium" || !f.CarryOver {
		t.Fatalf("foundation = %+v", f)
	}

	for _, year := range []int{1900, 2999} {
		if _, err := NewFoundation(year, "1", "x", false); err != nil {
			t.Fatalf("year %d must be accepted: %v", year, err)
		}
	}

	if _, err := NewFoundation(3000, "1", "x", false); !errors.Is(err, ErrBadYear) {
		t.Fatalf("bad year: want ErrBadYear, got %v", err)
	}
	if _, err := NewFoundation(2026, "", "x", false); !errors.Is(err, ErrNumberRequired) {
		t.Fatalf("blank number: want ErrNumberRequired, got %v", err)
	}
	if _, err := NewFoundation(2026, "1", "   ", false); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("blank description: want ErrNameRequired, got %v", err)
	}
}

// ── service level ───────────────────────────────────────────────────────────

type fakeTaxRepo struct {
	vat        VATType
	office     TaxOffice
	foundation Foundation
	err        error
}

func (f *fakeTaxRepo) CreateVATType(_ context.Context, v VATType) (VATType, error) {
	if f.err != nil {
		return VATType{}, f.err
	}
	v.ID = 5
	f.vat = v
	return v, nil
}

func (f *fakeTaxRepo) UpsertVATType(_ context.Context, v VATType) (VATType, error) {
	if f.err != nil {
		return VATType{}, f.err
	}
	v.ID = 5
	f.vat = v
	return v, nil
}

func (f *fakeTaxRepo) GetVATType(context.Context, string) (VATType, error) {
	if f.err != nil {
		return VATType{}, f.err
	}
	return f.vat, nil
}

func (f *fakeTaxRepo) CreateTaxOffice(_ context.Context, o TaxOffice) (TaxOffice, error) {
	if f.err != nil {
		return TaxOffice{}, f.err
	}
	o.ID = 6
	f.office = o
	return o, nil
}

func (f *fakeTaxRepo) UpsertTaxOffice(_ context.Context, o TaxOffice) (TaxOffice, error) {
	if f.err != nil {
		return TaxOffice{}, f.err
	}
	o.ID = 6
	f.office = o
	return o, nil
}

func (f *fakeTaxRepo) UpsertFoundation(_ context.Context, fo Foundation) (Foundation, error) {
	if f.err != nil {
		return Foundation{}, f.err
	}
	fo.ID = 7
	f.foundation = fo
	return fo, nil
}

func (f *fakeTaxRepo) CreateFoundation(_ context.Context, fo Foundation) (Foundation, error) {
	if f.err != nil {
		return Foundation{}, f.err
	}
	fo.ID = 7
	f.foundation = fo
	return fo, nil
}

func TestTaxServiceWritesAreAudited(t *testing.T) {
	ctx := context.Background()
	repo := &fakeTaxRepo{}
	sink := &fakeAudit{}
	svc := NewTaxService(repo, sink)

	vat, err := svc.CreateVATType(ctx, VATType{Code: "19", RateBasisPoints: 1900, Kind: VATOutput}, "alice")
	if err != nil {
		t.Fatalf("CreateVATType: %v", err)
	}
	if vat.ID != 5 {
		t.Fatalf("id = %d", vat.ID)
	}

	office, err := NewTaxOffice(2026, "1", "Amt", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("NewTaxOffice: %v", err)
	}
	if _, err := svc.CreateTaxOffice(ctx, office, "alice"); err != nil {
		t.Fatalf("CreateTaxOffice: %v", err)
	}

	foundation, err := NewFoundation(2026, "1", "Stipendium", false)
	if err != nil {
		t.Fatalf("NewFoundation: %v", err)
	}
	if _, err := svc.CreateFoundation(ctx, foundation, "alice"); err != nil {
		t.Fatalf("CreateFoundation: %v", err)
	}

	if len(sink.events) != 3 {
		t.Fatalf("audit events = %d, want 3", len(sink.events))
	}
	want := []string{"vat_type.create", "tax_office.create", "foundation.create"}
	for i, a := range want {
		if sink.events[i].Action != a {
			t.Fatalf("event %d = %q, want %q", i, sink.events[i].Action, a)
		}
	}
	if sink.events[0].Subject != "vat-type/19" {
		t.Fatalf("subject = %q", sink.events[0].Subject)
	}

	got, err := svc.VATType(ctx, "19")
	if err != nil {
		t.Fatalf("VATType: %v", err)
	}
	if got.Code != "19" {
		t.Fatalf("code = %q", got.Code)
	}
}

func TestTaxServiceErrorPaths(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	if _, err := NewTaxService(&fakeTaxRepo{}, &fakeAudit{}).CreateVATType(ctx, VATType{Code: "19"}, "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{}, &fakeAudit{}).CreateTaxOffice(ctx, TaxOffice{}, "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{}, &fakeAudit{}).CreateFoundation(ctx, Foundation{}, ""); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}

	if _, err := NewTaxService(&fakeTaxRepo{err: boom}, &fakeAudit{}).CreateVATType(ctx, VATType{Code: "19"}, "a"); !errors.Is(err, boom) {
		t.Fatalf("repo error: want boom, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{err: boom}, &fakeAudit{}).CreateTaxOffice(ctx, TaxOffice{}, "a"); !errors.Is(err, boom) {
		t.Fatalf("repo error: want boom, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{err: boom}, &fakeAudit{}).CreateFoundation(ctx, Foundation{}, "a"); !errors.Is(err, boom) {
		t.Fatalf("repo error: want boom, got %v", err)
	}

	if _, err := NewTaxService(&fakeTaxRepo{}, failingAudit{err: boom}).CreateVATType(ctx, VATType{Code: "19"}, "a"); !errors.Is(err, boom) {
		t.Fatalf("audit error: want boom, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{}, failingAudit{err: boom}).CreateTaxOffice(ctx, TaxOffice{}, "a"); !errors.Is(err, boom) {
		t.Fatalf("audit error: want boom, got %v", err)
	}
	if _, err := NewTaxService(&fakeTaxRepo{}, failingAudit{err: boom}).CreateFoundation(ctx, Foundation{}, "a"); !errors.Is(err, boom) {
		t.Fatalf("audit error: want boom, got %v", err)
	}
}
