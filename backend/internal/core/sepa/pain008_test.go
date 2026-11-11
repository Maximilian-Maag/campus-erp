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
				DebtorBIC:   "NWBKGB2L",
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
		"<Prtry>SEPA</Prtry>",
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
	// DbtrAgt is mandatory in pain.008: one block per transaction, before Dbtr.
	if n := strings.Count(xml, "<DbtrAgt>"); n != 2 {
		t.Errorf("DbtrAgt blocks = %d, want 2", n)
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
		{"missing debtor bic", func(d *DirectDebit) { d.Transactions[0].DebtorBIC = "" }, ErrDebtorAgentRequired},
		{"blank debtor bic", func(d *DirectDebit) { d.Transactions[0].DebtorBIC = "  " }, ErrDebtorAgentRequired},
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

// TestGoldenValidatesAgainstVendorXSD validates the golden pain.008.001.08 (GBIC) file against the
// vendor schema when ERP_XSD_PAIN008 points at it. The schema is proprietary and is not
// committed, so the check skips when the variable is unset (CI stays green); set it
// to the extracted XSD to actually validate.
func TestPain008ValidatesAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_PAIN008", filepath.Join("testdata", "golden_pain008.xml"))
}

// fixedDirectDebitBatch is a deterministic input with two payment instructions — one per sequence
// type. A collection run that mixes a first collection with a recurring one produces this shape: one
// message, two PmtInf blocks, each with its own sequence type and collection date.
func fixedDirectDebitBatch() DirectDebitBatch {
	single := fixedDirectDebit()
	batch := single.Batch()
	batch.Groups = []DirectDebitGroup{
		{
			InstructionID:  "SDD-2026-0001-FRST",
			SequenceType:   "FRST",
			CollectionDate: single.CollectionDate,
			Transactions:   []DirectDebitTransaction{single.Transactions[0]},
		},
		{
			InstructionID:  "SDD-2026-0001-RCUR",
			SequenceType:   "RCUR",
			CollectionDate: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
			Transactions:   []DirectDebitTransaction{single.Transactions[1]},
		},
	}
	return batch
}

// TestMarshal008BatchGolden pins the bytes of a message with two payment instructions: the shape a
// mixed run produces. Regenerate with UPDATE_GOLDEN=1.
func TestMarshal008BatchGolden(t *testing.T) {
	got, err := fixedDirectDebitBatch().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_pain008_two_groups.xml"), []byte(got))
}

// TestPain008TwoGroupsValidateAgainstVendorXSD checks the two-instruction file against the vendor
// schema too: several PmtInf blocks in one message is the part a bank verifies most strictly, so it
// is not enough that the single-instruction file validates.
func TestPain008TwoGroupsValidateAgainstVendorXSD(t *testing.T) {
	golden.ValidateXSD(t, "", "ERP_XSD_PAIN008", filepath.Join("testdata", "golden_pain008_two_groups.xml"))
}

// TestMarshal008BatchSplitsInstructions reads the produced XML: one PmtInf per group, each carrying
// its own sequence type, collection date and control sum, under a header that totals them.
func TestMarshal008BatchSplitsInstructions(t *testing.T) {
	got, err := fixedDirectDebitBatch().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if n := strings.Count(got, "<PmtInf>"); n != 2 {
		t.Fatalf("PmtInf blocks = %d, want 2\n%s", n, got)
	}
	// The header totals what the instructions carry.
	header := got[:strings.Index(got, "<PmtInf>")]
	if !strings.Contains(header, "<NbOfTxs>2</NbOfTxs>") {
		t.Errorf("header must total two transactions:\n%s", header)
	}
	if !strings.Contains(header, "<CtrlSum>55.00</CtrlSum>") {
		t.Errorf("header must total 55.00:\n%s", header)
	}
	// Each instruction carries its own type, date and sum — the reason SeqTp moved off the message.
	blocks := strings.Split(got, "<PmtInf>")[1:]
	for i, want := range []struct {
		id, seqTp, date, nb, ctrl string
	}{
		{"SDD-2026-0001-FRST", "FRST", "2026-10-28", "1", "42.50"},
		{"SDD-2026-0001-RCUR", "RCUR", "2026-11-27", "1", "12.50"},
	} {
		block := blocks[i]
		for _, pair := range [][2]string{
			{"<PmtInfId>", want.id},
			{"SeqTp>", want.seqTp},
			{"<ReqdColltnDt>", want.date},
			{"<NbOfTxs>", want.nb},
			{"<CtrlSum>", want.ctrl},
		} {
			if !strings.Contains(block, pair[0]) || !strings.Contains(block, pair[0]+pair[1]) {
				t.Errorf("instruction %d is missing %s%s:\n%s", i, pair[0], pair[1], block)
			}
		}
		// The creditor block repeats per instruction: a bank reads it per PmtInf.
		if strings.Count(block, "<CdtrSchmeId>") != 1 {
			t.Errorf("instruction %d must repeat the creditor:\n%s", i, block)
		}
	}
}

func TestMarshal008BatchCountAndSum(t *testing.T) {
	batch := fixedDirectDebitBatch()
	if got := batch.Count(); got != 2 {
		t.Errorf("Count() = %d, want 2", got)
	}
	if got := batch.Sum(); got != 5500 {
		t.Errorf("Sum() = %d, want 5500", got)
	}
	// One instruction is the one-instruction case: the general path must agree with the direct one.
	single := fixedDirectDebit()
	if got := single.Batch().Count(); got != len(single.Transactions) {
		t.Errorf("Batch().Count() = %d, want %d", got, len(single.Transactions))
	}
	if got := single.Batch().Sum(); got != single.Sum() {
		t.Errorf("Batch().Sum() = %d, want %d", got, single.Sum())
	}
}

func TestMarshal008BatchRejections(t *testing.T) {
	base := func() DirectDebitBatch { return fixedDirectDebitBatch() }

	t.Run("no instructions at all", func(t *testing.T) {
		b := base()
		b.Groups = nil
		if _, err := b.Marshal(); !errors.Is(err, ErrNoTransactions) {
			t.Fatalf("err = %v, want ErrNoTransactions", err)
		}
	})
	t.Run("an instruction without transactions", func(t *testing.T) {
		b := base()
		b.Groups[0].Transactions = nil
		if _, err := b.Marshal(); !errors.Is(err, ErrNoTransactions) {
			t.Fatalf("err = %v, want ErrNoTransactions", err)
		}
	})
	t.Run("two instructions sharing one id", func(t *testing.T) {
		b := base()
		b.Groups[1].InstructionID = b.Groups[0].InstructionID
		if _, err := b.Marshal(); !errors.Is(err, ErrInstructionIDDuplicate) {
			t.Fatalf("err = %v, want ErrInstructionIDDuplicate", err)
		}
	})
	t.Run("an instruction without a sequence type", func(t *testing.T) {
		b := base()
		b.Groups[1].SequenceType = ""
		if _, err := b.Marshal(); !errors.Is(err, ErrSequenceTypeInvalid) {
			t.Fatalf("err = %v, want ErrSequenceTypeInvalid", err)
		}
	})
	t.Run("an instruction without a collection date", func(t *testing.T) {
		b := base()
		b.Groups[1].CollectionDate = time.Time{}
		if _, err := b.Marshal(); !errors.Is(err, ErrCollectionDateRequired) {
			t.Fatalf("err = %v, want ErrCollectionDateRequired", err)
		}
	})
	t.Run("a transaction the single-place rules would refuse", func(t *testing.T) {
		// The per-transaction checks travel with the batch, so an instruction cannot smuggle in a
		// transaction the one-instruction path rejects.
		b := base()
		b.Groups[1].Transactions[0].DebtorBIC = ""
		if _, err := b.Marshal(); !errors.Is(err, ErrDebtorAgentRequired) {
			t.Fatalf("err = %v, want ErrDebtorAgentRequired", err)
		}
	})
	t.Run("a missing instruction id is allowed", func(t *testing.T) {
		// PmtInfId is not required by the schema, so an unnamed instruction is not a duplicate.
		b := base()
		b.Groups[0].InstructionID = ""
		b.Groups[1].InstructionID = ""
		if _, err := b.Marshal(); err != nil {
			t.Fatalf("err = %v, want none", err)
		}
	})
}
