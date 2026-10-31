package sepa

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

// fixedDirectDebit is a deterministic input used to pin the golden file.
func fixedDirectDebit() DirectDebit {
	return DirectDebit{
		MessageID:       "SDD-2026-0001",
		CreationTime:    time.Date(2026, 10, 26, 9, 30, 0, 0, time.UTC),
		InitiatingParty: "Universitaet Musterstadt",
		CreditorName:    "Universitaet Musterstadt",
		CreditorIBAN:    "DE02120300000000202051",
		CreditorBIC:     "BYLADEM1001",
		CreditorID:      "DE98ZZZ09999999999",
		SequenceType:    "RCUR",
		LocalInstrument: "CORE",
		CollectionDate:  time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
		Currency:        "EUR",
		Transactions: []DirectDebitTransaction{
			{
				EndToEndID:  "MAND-2026-0001",
				AmountCents: 4250,
				MandateID:   "MAN-2026-0001",
				MandateDate: time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC),
				DebtorName:  "Max Mustermann",
				DebtorIBAN:  "DE89370400440532013000",
				DebtorBIC:   "COBADEFFXXX",
				Purpose:     "Semesterbeitrag",
			},
			{
				EndToEndID:  "MAND-2026-0002",
				AmountCents: 1250,
				MandateID:   "MAN-2026-0002",
				MandateDate: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC),
				DebtorName:  "Erika Musterfrau",
				DebtorIBAN:  "GB29NWBK60161331926819",
				Purpose:     "Bibliotheksgebuehr",
			},
		},
	}
}

// TestMarshal008Golden pins the exact bytes of a pain.008 message. Regenerate with
// UPDATE_GOLDEN=1.
func TestMarshal008Golden(t *testing.T) {
	got, err := fixedDirectDebit().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_pain008.xml"), []byte(got))
}

func TestMarshal008Deterministic(t *testing.T) {
	a, err := fixedDirectDebit().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fixedDirectDebit().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Marshal is not deterministic")
	}
}

func TestMarshal008Structure(t *testing.T) {
	xml, err := fixedDirectDebit().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<Document xmlns="` + Pain008Namespace + `"`,
		"<MsgId>SDD-2026-0001</MsgId>",
		"<CreDtTm>2026-10-26T09:30:00</CreDtTm>",
		"<NbOfTxs>2</NbOfTxs>",
		"<CtrlSum>55.00</CtrlSum>",
		"<PmtMtd>DD</PmtMtd>",
		"<Cd>SEPA</Cd>",
		"<Cd>CORE</Cd>",
		"<SeqTp>RCUR</SeqTp>",
		"<ReqdColltnDt>2026-10-28</ReqdColltnDt>",
		"<ChrgBr>SLEV</ChrgBr>",
		"<Id>DE98ZZZ09999999999</Id>",
		`<InstdAmt Ccy="EUR">42.50</InstdAmt>`,
		`<InstdAmt Ccy="EUR">12.50</InstdAmt>`,
		"<MndtId>MAN-2026-0001</MndtId>",
		"<DtOfSgntr>2025-03-14</DtOfSgntr>",
		"<EndToEndId>MAND-2026-0001</EndToEndId>",
		"<Ustrd>Semesterbeitrag</Ustrd>",
		"</Document>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// The second transaction has no debtor BIC, so there is exactly one DbtrAgt.
	if n := strings.Count(xml, "<DbtrAgt>"); n != 1 {
		t.Errorf("DbtrAgt blocks = %d, want 1", n)
	}
	if n := strings.Count(xml, "<DrctDbtTxInf>"); n != 2 {
		t.Errorf("DrctDbtTxInf blocks = %d, want 2", n)
	}
}

func TestMarshal008EscapesText(t *testing.T) {
	dd := fixedDirectDebit()
	dd.Transactions[0].Purpose = `Miete & <Nebenkosten> "Q3"`
	xml, err := dd.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(xml, "<Ustrd>Miete &amp; &lt;Nebenkosten&gt; &quot;Q3&quot;</Ustrd>") {
		t.Fatalf("purpose not escaped:\n%s", xml)
	}
}

func TestSum008(t *testing.T) {
	if got := fixedDirectDebit().Sum(); got != 5500 {
		t.Fatalf("Sum = %d, want 5500", got)
	}
}

func TestMarshal008AtLengthLimits(t *testing.T) {
	dd := fixedDirectDebit()
	dd.MessageID = strings.Repeat("m", MaxText35)
	dd.InitiatingParty = strings.Repeat("i", MaxText140)
	dd.CreditorName = strings.Repeat("c", MaxText140)
	dd.CreditorID = strings.Repeat("r", MaxText35)
	dd.Transactions[0].EndToEndID = strings.Repeat("e", MaxText35)
	dd.Transactions[0].MandateID = strings.Repeat("d", MaxText35)
	dd.Transactions[0].DebtorName = strings.Repeat("n", MaxText140)
	dd.Transactions[0].Purpose = strings.Repeat("p", MaxText140)
	if _, err := dd.Marshal(); err != nil {
		t.Fatalf("values exactly at the limits must be accepted: %v", err)
	}
}

func TestCurrency008DefaultsToEUR(t *testing.T) {
	dd := fixedDirectDebit()
	dd.Currency = ""
	xml, err := dd.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(xml, `<InstdAmt Ccy="EUR">`) {
		t.Fatal("empty currency must default to EUR")
	}
}

func TestMarshal008Validation(t *testing.T) {
	valid := func() DirectDebit { return fixedDirectDebit() }

	cases := []struct {
		name   string
		mutate func(*DirectDebit)
		want   error
	}{
		{"empty message id", func(d *DirectDebit) { d.MessageID = "" }, ErrMessageIDRequired},
		{"blank message id", func(d *DirectDebit) { d.MessageID = "   " }, ErrMessageIDRequired},
		{"message id too long", func(d *DirectDebit) { d.MessageID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"zero creation time", func(d *DirectDebit) { d.CreationTime = time.Time{} }, ErrCreationTimeRequired},
		{"empty initiating party", func(d *DirectDebit) { d.InitiatingParty = "" }, ErrInitiatingPartyRequired},
		{"initiating party too long", func(d *DirectDebit) { d.InitiatingParty = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"creditor name too long", func(d *DirectDebit) { d.CreditorName = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"empty creditor name", func(d *DirectDebit) { d.CreditorName = "" }, ErrCreditorNameRequired},
		{"bad creditor iban", func(d *DirectDebit) { d.CreditorIBAN = "DE00" }, ErrCreditorIBANInvalid},
		{"bad creditor bic", func(d *DirectDebit) { d.CreditorBIC = "nope" }, ErrCreditorBICInvalid},
		{"empty creditor id", func(d *DirectDebit) { d.CreditorID = " " }, ErrCreditorIDRequired},
		{"creditor id too long", func(d *DirectDebit) { d.CreditorID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"bad sequence type", func(d *DirectDebit) { d.SequenceType = "X" }, ErrSequenceTypeInvalid},
		{"bad local instrument", func(d *DirectDebit) { d.LocalInstrument = "COR1" }, ErrLocalInstrumentInvalid},
		{"bad currency", func(d *DirectDebit) { d.Currency = "EURO" }, ErrCurrencyInvalid},
		{"zero collection date", func(d *DirectDebit) { d.CollectionDate = time.Time{} }, ErrCollectionDateRequired},
		{"no transactions", func(d *DirectDebit) { d.Transactions = nil }, ErrNoTransactions},
		{"empty end-to-end id", func(d *DirectDebit) { d.Transactions[0].EndToEndID = "" }, ErrEndToEndIDRequired},
		{"end-to-end id too long", func(d *DirectDebit) { d.Transactions[0].EndToEndID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"debtor name too long", func(d *DirectDebit) { d.Transactions[0].DebtorName = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"purpose too long", func(d *DirectDebit) { d.Transactions[0].Purpose = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"non-positive amount", func(d *DirectDebit) { d.Transactions[0].AmountCents = 0 }, ErrAmountNotPositive},
		{"empty mandate id", func(d *DirectDebit) { d.Transactions[0].MandateID = "" }, ErrMandateIDRequired},
		{"mandate id too long", func(d *DirectDebit) { d.Transactions[0].MandateID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"zero mandate date", func(d *DirectDebit) { d.Transactions[0].MandateDate = time.Time{} }, ErrMandateDateRequired},
		{"empty debtor name", func(d *DirectDebit) { d.Transactions[0].DebtorName = "" }, ErrDebtorNameRequired},
		{"bad debtor iban", func(d *DirectDebit) { d.Transactions[0].DebtorIBAN = "XX" }, ErrDebtorIBANInvalid},
		{"bad debtor bic", func(d *DirectDebit) { d.Transactions[0].DebtorBIC = "12345678" }, ErrDebtorBICInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dd := valid()
			c.mutate(&dd)
			if _, err := dd.Marshal(); !errors.Is(err, c.want) {
				t.Fatalf("Marshal error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestValidSequenceType(t *testing.T) {
	for _, in := range []string{"FRST", "RCUR", "OOFF", "FNAL"} {
		if !validSequenceType(in) {
			t.Errorf("validSequenceType(%q) = false", in)
		}
	}
	for _, in := range []string{"", "rcur", "OOFF ", "X"} {
		if validSequenceType(in) {
			t.Errorf("validSequenceType(%q) = true", in)
		}
	}
}

func TestValidLocalInstrument(t *testing.T) {
	for _, in := range []string{"CORE", "B2B"} {
		if !validLocalInstrument(in) {
			t.Errorf("validLocalInstrument(%q) = false", in)
		}
	}
	for _, in := range []string{"", "core", "B2C"} {
		if validLocalInstrument(in) {
			t.Errorf("validLocalInstrument(%q) = true", in)
		}
	}
}
