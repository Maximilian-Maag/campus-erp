package sepa

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
)

// fixedCreditTransfer is a deterministic input used to pin the golden file.
func fixedCreditTransfer() CreditTransfer {
	return CreditTransfer{
		MessageID:       "RUN-2026-0001",
		CreationTime:    time.Date(2026, 10, 26, 9, 30, 0, 0, time.UTC),
		InitiatingParty: "Universitaet Musterstadt",
		DebtorName:      "Universitaet Musterstadt",
		DebtorIBAN:      "DE02120300000000202051",
		DebtorBIC:       "BYLADEM1001",
		Currency:        "EUR",
		ExecutionDate:   time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
		Transactions: []Transaction{
			{
				EndToEndID:   "AO-2026-0001",
				AmountCents:  125000,
				CreditorName: "Stadtwerke Musterstadt",
				CreditorIBAN: "DE89370400440532013000",
				CreditorBIC:  "COBADEFFXXX",
				Purpose:      "Stromabschlag Oktober",
			},
			{
				EndToEndID:   "AO-2026-0002",
				AmountCents:  4999,
				CreditorName: "Buchhandlung Am Markt",
				CreditorIBAN: "GB29NWBK60161331926819",
				Purpose:      "Fachliteratur",
			},
		},
	}
}

// TestMarshalGolden pins the exact bytes of a pain.001 message. Regenerate with
// UPDATE_GOLDEN=1.
func TestMarshalGolden(t *testing.T) {
	got, err := fixedCreditTransfer().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_pain001.xml"), []byte(got))
}

// TestMarshalDeterministic guards the "byte-identical" requirement: the same
// input must always produce the same bytes.
func TestMarshalDeterministic(t *testing.T) {
	a, err := fixedCreditTransfer().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fixedCreditTransfer().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Marshal is not deterministic")
	}
}

func TestMarshalStructure(t *testing.T) {
	xml, err := fixedCreditTransfer().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<Document xmlns="` + Pain001Namespace + `"`,
		"<MsgId>RUN-2026-0001</MsgId>",
		"<CreDtTm>2026-10-26T09:30:00</CreDtTm>",
		"<NbOfTxs>2</NbOfTxs>",
		"<CtrlSum>1299.99</CtrlSum>",
		"<PmtMtd>TRF</PmtMtd>",
		"<BtchBookg>true</BtchBookg>",
		"<Cd>SEPA</Cd>",
		"<Dt>2026-10-28</Dt>",
		`<InstdAmt Ccy="EUR">1250.00</InstdAmt>`,
		`<InstdAmt Ccy="EUR">49.99</InstdAmt>`,
		"<ChrgBr>SLEV</ChrgBr>",
		"<EndToEndId>AO-2026-0001</EndToEndId>",
		"<BICFI>BYLADEM1001</BICFI>",
		"<Ustrd>Stromabschlag Oktober</Ustrd>",
		"</Document>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("output missing %q", want)
		}
	}
	// The second transaction has no BIC, so there must be exactly one creditor
	// agent block (for the first transaction) and one debtor agent block.
	if n := strings.Count(xml, "<CdtrAgt>"); n != 1 {
		t.Errorf("CdtrAgt blocks = %d, want 1", n)
	}
	if n := strings.Count(xml, "<DbtrAgt>"); n != 1 {
		t.Errorf("DbtrAgt blocks = %d, want 1", n)
	}
}

func TestMarshalEscapesText(t *testing.T) {
	ct := fixedCreditTransfer()
	ct.Transactions[0].Purpose = `Miete & <Nebenkosten> "Q3"`
	xml, err := ct.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xml, "<Ustrd>Miete &amp; &lt;Nebenkosten&gt; &quot;Q3&quot;</Ustrd>") {
		t.Fatalf("purpose not escaped:\n%s", xml)
	}
}

func TestSum(t *testing.T) {
	if got := fixedCreditTransfer().Sum(); got != 129999 {
		t.Fatalf("Sum = %d, want 129999", got)
	}
}

// TestMarshalAtLengthLimits kills the boundary mutants on the Max35/Max140 checks:
// a value exactly at the limit must be accepted.
func TestMarshalAtLengthLimits(t *testing.T) {
	ct := fixedCreditTransfer()
	ct.MessageID = strings.Repeat("m", MaxText35)
	ct.InitiatingParty = strings.Repeat("i", MaxText140)
	ct.DebtorName = strings.Repeat("d", MaxText140)
	ct.Transactions[0].EndToEndID = strings.Repeat("e", MaxText35)
	ct.Transactions[0].CreditorName = strings.Repeat("c", MaxText140)
	ct.Transactions[0].Purpose = strings.Repeat("p", MaxText140)
	if _, err := ct.Marshal(); err != nil {
		t.Fatalf("values exactly at the limits must be accepted: %v", err)
	}
}

func TestCurrencyDefaultsToEUR(t *testing.T) {
	ct := fixedCreditTransfer()
	ct.Currency = ""
	xml, err := ct.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(xml, "<Ccy>EUR</Ccy>") {
		t.Fatal("empty currency must default to EUR")
	}
}

// TestMarshalValidation covers every validation branch.
func TestMarshalValidation(t *testing.T) {
	valid := func() CreditTransfer { return fixedCreditTransfer() }

	cases := []struct {
		name   string
		mutate func(*CreditTransfer)
		want   error
	}{
		{"empty message id", func(c *CreditTransfer) { c.MessageID = "" }, ErrMessageIDRequired},
		{"blank message id", func(c *CreditTransfer) { c.MessageID = "   " }, ErrMessageIDRequired},
		{"message id too long", func(c *CreditTransfer) { c.MessageID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"zero creation time", func(c *CreditTransfer) { c.CreationTime = time.Time{} }, ErrCreationTimeRequired},
		{"empty initiating party", func(c *CreditTransfer) { c.InitiatingParty = "" }, ErrInitiatingPartyRequired},
		{"initiating party too long", func(c *CreditTransfer) { c.InitiatingParty = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"debtor name too long", func(c *CreditTransfer) { c.DebtorName = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"empty debtor name", func(c *CreditTransfer) { c.DebtorName = "" }, ErrDebtorNameRequired},
		{"bad debtor iban", func(c *CreditTransfer) { c.DebtorIBAN = "DE00" }, ErrDebtorIBANInvalid},
		{"bad debtor bic", func(c *CreditTransfer) { c.DebtorBIC = "nope" }, ErrDebtorBICInvalid},
		{"bad currency", func(c *CreditTransfer) { c.Currency = "EURO" }, ErrCurrencyInvalid},
		{"zero execution date", func(c *CreditTransfer) { c.ExecutionDate = time.Time{} }, ErrExecutionDateRequired},
		{"no transactions", func(c *CreditTransfer) { c.Transactions = nil }, ErrNoTransactions},
		{"empty end-to-end id", func(c *CreditTransfer) { c.Transactions[0].EndToEndID = "" }, ErrEndToEndIDRequired},
		{"end-to-end id too long", func(c *CreditTransfer) { c.Transactions[0].EndToEndID = strings.Repeat("x", 36) }, ErrFieldTooLong},
		{"creditor name too long", func(c *CreditTransfer) { c.Transactions[0].CreditorName = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"purpose too long", func(c *CreditTransfer) { c.Transactions[0].Purpose = strings.Repeat("x", 141) }, ErrFieldTooLong},
		{"non-positive amount", func(c *CreditTransfer) { c.Transactions[0].AmountCents = 0 }, ErrAmountNotPositive},
		{"empty creditor name", func(c *CreditTransfer) { c.Transactions[0].CreditorName = "" }, ErrCreditorNameRequired},
		{"bad creditor iban", func(c *CreditTransfer) { c.Transactions[0].CreditorIBAN = "XX" }, ErrCreditorIBANInvalid},
		{"bad creditor bic", func(c *CreditTransfer) { c.Transactions[0].CreditorBIC = "12345678" }, ErrCreditorBICInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct := valid()
			c.mutate(&ct)
			if _, err := ct.Marshal(); !errors.Is(err, c.want) {
				t.Fatalf("Marshal error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestValidBIC(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"COBADEFFXXX", true},
		{"cobadeffxxx", true},
		{"COBADEFF", true},
		{"COBADEFFX", false},
		{"COBADEFFXXXX", false},
		{"12345678", false},
		{"COBADEFF1XX", true},
		{"COBADEFF!XX", false},
		{"COBADE1F", true},
		{"", false},
	}
	for _, c := range cases {
		if got := ValidBIC(c.in); got != c.want {
			t.Errorf("ValidBIC(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidCurrency(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"EUR", true},
		{"eur", false},
		{"EU", false},
		{"EURO", false},
		{"EU1", false},
	}
	for _, c := range cases {
		if got := validCurrency(c.in); got != c.want {
			t.Errorf("validCurrency(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestGoldenValidatesAgainstVendorXSD validates the golden pain.001.001.09 (DK/GBIC) file against the
// vendor schema when ERP_XSD_PAIN001 points at it. The schema is proprietary and is not
// committed, so the check skips when the variable is unset (CI stays green); set it
// to the extracted XSD to actually validate.
func TestPain001ValidatesAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_PAIN001", filepath.Join("testdata", "golden_pain001.xml"))
}
