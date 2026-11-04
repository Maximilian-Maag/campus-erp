package kasse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewReceivableValidation(t *testing.T) {
	due := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	// A well-formed item passes and is normalised (currency upper-cased, fields trimmed).
	rec, err := NewReceivable("  MAN-1 ", 4250, "eur", due, "  Semesterbeitrag  ", "  dunning  ")
	if err != nil {
		t.Fatalf("valid receivable rejected: %v", err)
	}
	if rec.MandateReference != "MAN-1" || rec.Currency != "EUR" || rec.Purpose != "Semesterbeitrag" ||
		rec.Source != "dunning" || rec.State != ReceivableOpen || rec.AmountCents != 4250 {
		t.Fatalf("receivable not normalised: %+v", rec)
	}

	// The length bounds are inclusive: a mandate reference of exactly 35 and a
	// purpose of exactly 140 runes are accepted; only beyond them is refused.
	atLimit, err := NewReceivable(strings.Repeat("M", maxMandateRefLen), 100, "EUR", due,
		strings.Repeat("p", maxPurposeLen), "")
	if err != nil {
		t.Fatalf("at-limit receivable rejected: %v", err)
	}
	if len(atLimit.MandateReference) != maxMandateRefLen || len([]rune(atLimit.Purpose)) != maxPurposeLen {
		t.Fatalf("at-limit receivable changed: %+v", atLimit)
	}

	longPurpose := strings.Repeat("a", 141)
	cases := []struct {
		name    string
		ref     string
		amount  int64
		cur     string
		due     time.Time
		purpose string
		want    error
	}{
		{"blank mandate", "  ", 100, "EUR", due, "x", ErrMandateReferenceRequired},
		{"mandate too long", strings.Repeat("M", 36), 100, "EUR", due, "x", ErrMandateReferenceTooLong},
		{"zero amount", "MAN-1", 0, "EUR", due, "x", ErrReceivableAmountNotPositive},
		{"negative amount", "MAN-1", -5, "EUR", due, "x", ErrReceivableAmountNotPositive},
		{"short currency", "MAN-1", 100, "EU", due, "x", ErrReceivableCurrencyRequired},
		{"blank currency", "MAN-1", 100, " ", due, "x", ErrReceivableCurrencyRequired},
		{"no due date", "MAN-1", 100, "EUR", time.Time{}, "x", ErrReceivableDueDateRequired},
		{"blank purpose", "MAN-1", 100, "EUR", due, "   ", ErrReceivablePurposeRequired},
		{"purpose too long", "MAN-1", 100, "EUR", due, longPurpose, ErrReceivablePurposeTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewReceivable(c.ref, c.amount, c.cur, c.due, c.purpose, ""); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestCollectionItemsFromReceivables(t *testing.T) {
	recs := []Receivable{
		{ID: 10, MandateReference: "MAN-B", AmountCents: 1000, Purpose: "Gebuehr"},
		{ID: 11, MandateReference: "MAN-A", AmountCents: 2000, Purpose: "Beitrag"},
		{ID: 12, MandateReference: "MAN-B", AmountCents: 500, Purpose: "Gebuehr"}, // same purpose deduped
		{ID: 13, MandateReference: "MAN-A", AmountCents: 250, Purpose: "Miete"},
	}
	items, claims := CollectionItemsFromReceivables(recs)

	// One item per mandate, mandates in a stable order.
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].MandateReference != "MAN-A" || items[1].MandateReference != "MAN-B" {
		t.Fatalf("mandates not sorted: %+v", items)
	}
	// A mandate's items are summed, its end-to-end id is the mandate reference.
	if items[0].AmountCents != 2250 || items[0].EndToEndID != "MAN-A" {
		t.Fatalf("MAN-A item = %+v, want 2250 / MAN-A", items[0])
	}
	if items[1].AmountCents != 1500 {
		t.Fatalf("MAN-B item = %+v, want 1500", items[1])
	}
	// Purposes are joined in order, duplicates dropped.
	if items[0].Purpose != "Beitrag; Miete" || items[1].Purpose != "Gebuehr" {
		t.Fatalf("purposes = %q / %q", items[0].Purpose, items[1].Purpose)
	}
	// The claims map every mandate to exactly the ids it contributed.
	if got := claims["MAN-A"]; len(got) != 2 || got[0] != 11 || got[1] != 13 {
		t.Fatalf("claims[MAN-A] = %v", got)
	}
	if got := claims["MAN-B"]; len(got) != 2 || got[0] != 10 || got[1] != 12 {
		t.Fatalf("claims[MAN-B] = %v", got)
	}
}

func TestCollectionItemsFromReceivablesEmpty(t *testing.T) {
	items, claims := CollectionItemsFromReceivables(nil)
	if len(items) != 0 || len(claims) != 0 {
		t.Fatalf("empty input gave %d items, %d claims", len(items), len(claims))
	}
}

func TestJoinPurposesTruncatesAtRunes(t *testing.T) {
	// 200 distinct purposes join well past 140; the result is capped, and a
	// two-byte rune at the cut is never split.
	many := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		many = append(many, fmt.Sprintf("%dü", i))
	}
	got := joinPurposes(many)
	if len([]rune(got)) != maxPurposeLen {
		t.Fatalf("length = %d runes, want %d", len([]rune(got)), maxPurposeLen)
	}
	// Every rune is complete: a mid-rune cut would decode as U+FFFD.
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("truncation split a rune: %q", got)
	}
	// A short join is returned untouched; blanks are dropped and duplicates removed.
	if s := joinPurposes([]string{"a", "  ", "a", "b"}); s != "a; b" {
		t.Fatalf("join = %q, want %q", s, "a; b")
	}
}
