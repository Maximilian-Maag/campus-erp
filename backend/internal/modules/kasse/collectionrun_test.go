package kasse

import (
	"errors"
	"testing"
	"time"
)

func TestNewCollectionRunValidation(t *testing.T) {
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	one := func() []CollectionItem {
		return []CollectionItem{{MandateReference: "MAN-1", EndToEndID: "E1", AmountCents: 1000, Purpose: "Beitrag"}}
	}
	// A well-formed run passes.
	if _, items, err := NewCollectionRun("RUN-1", 1, "DE98ZZZ09999999999", date, SequenceRecurring, "CORE", one()); err != nil {
		t.Fatalf("valid run rejected: %v (%d items)", err, len(items))
	}
	// Items are trimmed.
	if _, items, err := NewCollectionRun("RUN-1", 1, "DE98ZZZ09999999999", date, SequenceRecurring, "CORE",
		[]CollectionItem{{MandateReference: "  MAN-1  ", EndToEndID: " E1 ", AmountCents: 1000, Purpose: " x "}}); err != nil {
		t.Fatalf("valid run rejected: %v", err)
	} else if items[0].MandateReference != "MAN-1" || items[0].Purpose != "x" {
		t.Fatalf("items not trimmed: %+v", items[0])
	}

	cases := []struct {
		name      string
		reference string
		account   int64
		creditor  string
		date      time.Time
		seq       MandateSequence
		li        string
		items     []CollectionItem
		want      error
	}{
		{"blank reference", "  ", 1, "CID", date, SequenceRecurring, "CORE", one(), ErrRunReferenceRequired},
		{"no account", "RUN", 0, "CID", date, SequenceRecurring, "CORE", one(), ErrCashAccountRequired},
		{"no creditor id", "RUN", 1, "  ", date, SequenceRecurring, "CORE", one(), ErrCollectionCreditorIDRequired},
		{"no date", "RUN", 1, "CID", time.Time{}, SequenceRecurring, "CORE", one(), ErrCollectionDateRequired},
		{"bad sequence", "RUN", 1, "CID", date, "X", "CORE", one(), ErrSequenceTypeUnknown},
		{"bad local instrument", "RUN", 1, "CID", date, SequenceRecurring, "COR1", one(), ErrLocalInstrumentUnknown},
		{"no items", "RUN", 1, "CID", date, SequenceRecurring, "CORE", nil, ErrNoCollectionItems},
		{"blank mandate", "RUN", 1, "CID", date, SequenceRecurring, "CORE",
			[]CollectionItem{{MandateReference: " ", EndToEndID: "E", AmountCents: 1}}, ErrMandateReferenceRequired},
		{"duplicate mandate", "RUN", 1, "CID", date, SequenceRecurring, "CORE",
			[]CollectionItem{{MandateReference: "M", EndToEndID: "E1", AmountCents: 1}, {MandateReference: "M", EndToEndID: "E2", AmountCents: 2}}, ErrDuplicateMandate},
		{"non-positive amount", "RUN", 1, "CID", date, SequenceRecurring, "CORE",
			[]CollectionItem{{MandateReference: "M", EndToEndID: "E", AmountCents: 0}}, ErrAmountNotPositive},
		{"blank end-to-end", "RUN", 1, "CID", date, SequenceRecurring, "CORE",
			[]CollectionItem{{MandateReference: "M", EndToEndID: "  ", AmountCents: 1}}, ErrEndToEndIDRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := NewCollectionRun(c.reference, c.account, c.creditor, c.date, c.seq, c.li, c.items); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
