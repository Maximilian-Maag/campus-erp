package kasse

import (
	"errors"
	"testing"
)

func TestNewCashAccount(t *testing.T) {
	cases := []struct {
		name     string
		accName  string
		iban     string
		bic      string
		currency string
		wantErr  error
		check    func(CashAccount) bool
	}{
		{name: "valid", accName: "Universität", iban: "DE02120300000000202051", bic: "BYLADEM1001", currency: "EUR",
			check: func(a CashAccount) bool { return a.IBAN == "DE02120300000000202051" && a.Currency == "EUR" }},
		{name: "normalises iban and bic", accName: "Uni", iban: "de02 1203 0000 0000 2020 51", bic: "byladem1001", currency: "eur",
			check: func(a CashAccount) bool { return a.IBAN == "DE02120300000000202051" && a.BIC == "BYLADEM1001" && a.Currency == "EUR" }},
		{name: "currency defaults to eur", accName: "Uni", iban: "DE02120300000000202051", currency: "",
			check: func(a CashAccount) bool { return a.Currency == "EUR" }},
		{name: "blank name", accName: "  ", iban: "DE02120300000000202051", currency: "EUR", wantErr: ErrRecipientRequired},
		{name: "bad iban", accName: "Uni", iban: "DE00", currency: "EUR", wantErr: ErrIBANInvalid},
		{name: "bad currency", accName: "Uni", iban: "DE02120300000000202051", currency: "EURO", wantErr: ErrBadCurrency},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewCashAccount(c.accName, c.iban, c.bic, c.currency)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.check(got) {
				t.Fatalf("account = %+v", got)
			}
		})
	}
}

func TestDedupe(t *testing.T) {
	cases := []struct {
		name string
		in   []int64
		want []int64
	}{
		{"empty", nil, []int64{}},
		{"no duplicates", []int64{3, 1, 2}, []int64{3, 1, 2}},
		{"duplicates", []int64{3, 1, 3, 1, 2}, []int64{3, 1, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dedupe(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("dedupe(%v) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("dedupe(%v) = %v, want %v", c.in, got, c.want)
				}
			}
		})
	}
}
