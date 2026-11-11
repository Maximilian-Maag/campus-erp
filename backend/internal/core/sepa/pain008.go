package sepa

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// This file generates a SEPA direct-debit initiation, ISO 20022
// pain.008.001.08 — the collection counterpart of pain001.go. It follows the
// same conventions: stdlib-only, deterministic output pinned by a golden file, and
// the element order of the XSD sequences.
//
// Element order:
//
//	GroupHeader83:            MsgId, CreDtTm, NbOfTxs, CtrlSum, InitgPty
//	PaymentInstruction29:     PmtInfId, PmtMtd, NbOfTxs, CtrlSum, PmtTpInf,
//	                          ReqdColltnDt, Cdtr, CdtrAcct, CdtrAgt, ChrgBr,
//	                          CdtrSchmeId, DrctDbtTxInf+
//	DirectDebitTransactionInformation23: PmtId, InstdAmt, DrctDbtTx, DbtrAgt,
//	                          Dbtr, DbtrAcct, RmtInf
//	MandateRelatedInformation14: MndtId, DtOfSgntr

const (
	// Pain008Namespace is the ISO 20022 namespace of pain.008.001.08.
	Pain008Namespace = "urn:iso:std:iso:20022:tech:xsd:pain.008.001.08"
	// Pain008SchemaLocation is the matching xsi:schemaLocation value.
	Pain008SchemaLocation = "urn:iso:std:iso:20022:tech:xsd:pain.008.001.08  pain.008.001.08.xsd"
)

// Validation errors. They are values so callers can match them with errors.Is.
var (
	ErrCreditorIDRequired     = errors.New("sepa: creditor identifier is required")
	ErrSequenceTypeInvalid    = errors.New("sepa: sequence type must be FRST, RCUR, OOFF or FNAL")
	ErrLocalInstrumentInvalid = errors.New("sepa: local instrument must be CORE or B2B")
	ErrCollectionDateRequired = errors.New("sepa: collection date is required")
	ErrMandateIDRequired      = errors.New("sepa: mandate id is required")
	ErrMandateDateRequired    = errors.New("sepa: mandate signature date is required")
	// ErrDebtorAgentRequired is returned when a transaction has no debtor BIC:
	// DbtrAgt is 1..1 in pain.008 (the GBIC profile enforces it), so a file without
	// it is rejected by the bank.
	ErrDebtorAgentRequired = errors.New("sepa: debtor BIC is required (DbtrAgt is mandatory)")
	// ErrInstructionIDDuplicate is returned when two payment instructions of one
	// message share an identifier, which would make the file ambiguous to the bank.
	ErrInstructionIDDuplicate = errors.New("sepa: payment instruction id is used twice")
)

// DirectDebitTransaction is one collection from one debtor under one mandate.
type DirectDebitTransaction struct {
	EndToEndID  string
	AmountCents int64
	MandateID   string
	MandateDate time.Time
	DebtorName  string
	DebtorIBAN  string
	DebtorBIC   string
	Purpose     string
}

// DirectDebit is a whole pain.008 message: one creditor collecting from many
// debtors on one collection date under one sequence type.
type DirectDebit struct {
	MessageID       string
	CreationTime    time.Time
	InitiatingParty string
	CreditorName    string
	CreditorIBAN    string
	CreditorBIC     string
	CreditorID      string // SEPA creditor identifier
	SequenceType    string // FRST / RCUR / OOFF / FNAL
	LocalInstrument string // CORE / B2B
	CollectionDate  time.Time
	Currency        string
	Transactions    []DirectDebitTransaction
}

// currency returns the effective currency, defaulting to EUR.
func (dd DirectDebit) currency() string {
	if dd.Currency == "" {
		return defaultCurrency
	}
	return dd.Currency
}

// Sum returns the total amount in cents.
func (dd DirectDebit) Sum() int64 {
	var sum int64
	for _, tx := range dd.Transactions {
		sum += tx.AmountCents
	}
	return sum
}

func validSequenceType(s string) bool {
	switch s {
	case "FRST", "RCUR", "OOFF", "FNAL":
		return true
	default:
		return false
	}
}

func validLocalInstrument(s string) bool {
	switch s {
	case "CORE", "B2B":
		return true
	default:
		return false
	}
}

// DirectDebitGroup is one payment instruction: everything a pain.008 `PmtInf` repeats. The sequence
// type lives here, not on the message, which is why a run that collects under more than one type is
// one message with several of these rather than several files.
type DirectDebitGroup struct {
	InstructionID  string
	SequenceType   string // FRST / RCUR / OOFF / FNAL
	CollectionDate time.Time
	Transactions   []DirectDebitTransaction
}

// sum returns the group's total in cents.
func (g DirectDebitGroup) sum() int64 {
	var sum int64
	for _, tx := range g.Transactions {
		sum += tx.AmountCents
	}
	return sum
}

// DirectDebitBatch is a whole pain.008 message: one creditor, one group header, and one or more
// payment instructions. It is the general form; `DirectDebit` is the one-instruction case.
type DirectDebitBatch struct {
	MessageID       string
	CreationTime    time.Time
	InitiatingParty string
	CreditorName    string
	CreditorIBAN    string
	CreditorBIC     string
	CreditorID      string // SEPA creditor identifier
	LocalInstrument string // CORE / B2B
	Currency        string
	Groups          []DirectDebitGroup
}

// currency returns the effective currency, defaulting to EUR.
func (b DirectDebitBatch) currency() string {
	if b.Currency == "" {
		return defaultCurrency
	}
	return b.Currency
}

// Sum returns the total amount in cents over every instruction.
func (b DirectDebitBatch) Sum() int64 {
	var sum int64
	for _, g := range b.Groups {
		sum += g.sum()
	}
	return sum
}

// Count returns the number of transactions over every instruction.
func (b DirectDebitBatch) Count() int {
	n := 0
	for _, g := range b.Groups {
		n += len(g.Transactions)
	}
	return n
}

// Batch expresses the one-instruction message as the general one, so there is a single writer and
// its output for this case stays byte-identical.
func (dd DirectDebit) Batch() DirectDebitBatch {
	return DirectDebitBatch{
		MessageID:       dd.MessageID,
		CreationTime:    dd.CreationTime,
		InitiatingParty: dd.InitiatingParty,
		CreditorName:    dd.CreditorName,
		CreditorIBAN:    dd.CreditorIBAN,
		CreditorBIC:     dd.CreditorBIC,
		CreditorID:      dd.CreditorID,
		LocalInstrument: dd.LocalInstrument,
		Currency:        dd.Currency,
		Groups: []DirectDebitGroup{{
			InstructionID:  dd.MessageID,
			SequenceType:   dd.SequenceType,
			CollectionDate: dd.CollectionDate,
			Transactions:   dd.Transactions,
		}},
	}
}

// validate checks every constraint the header, every instruction and every transaction must meet.
func (b DirectDebitBatch) validate() error {
	if strings.TrimSpace(b.MessageID) == "" {
		return ErrMessageIDRequired
	}
	if len(b.MessageID) > MaxText35 {
		return ErrFieldTooLong
	}
	if b.CreationTime.IsZero() {
		return ErrCreationTimeRequired
	}
	if strings.TrimSpace(b.InitiatingParty) == "" {
		return ErrInitiatingPartyRequired
	}
	if len(b.InitiatingParty) > MaxText140 || len(b.CreditorName) > MaxText140 {
		return ErrFieldTooLong
	}
	if strings.TrimSpace(b.CreditorName) == "" {
		return ErrCreditorNameRequired
	}
	if !ValidIBAN(b.CreditorIBAN) {
		return ErrCreditorIBANInvalid
	}
	if b.CreditorBIC != "" && !ValidBIC(b.CreditorBIC) {
		return ErrCreditorBICInvalid
	}
	if strings.TrimSpace(b.CreditorID) == "" {
		return ErrCreditorIDRequired
	}
	if len(b.CreditorID) > MaxText35 {
		return ErrFieldTooLong
	}
	if !validLocalInstrument(b.LocalInstrument) {
		return ErrLocalInstrumentInvalid
	}
	if !validCurrency(b.currency()) {
		return ErrCurrencyInvalid
	}
	if len(b.Groups) == 0 {
		return ErrNoTransactions
	}
	seen := make(map[string]bool, len(b.Groups))
	for _, g := range b.Groups {
		if !validSequenceType(g.SequenceType) {
			return ErrSequenceTypeInvalid
		}
		if len(g.InstructionID) > MaxText35 {
			return ErrFieldTooLong
		}
		// A repeated instruction identifier would make the file ambiguous to the bank.
		if g.InstructionID != "" {
			if seen[g.InstructionID] {
				return ErrInstructionIDDuplicate
			}
			seen[g.InstructionID] = true
		}
		if g.CollectionDate.IsZero() {
			return ErrCollectionDateRequired
		}
		if len(g.Transactions) == 0 {
			return ErrNoTransactions
		}
		for _, tx := range g.Transactions {
			if err := tx.validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tx DirectDebitTransaction) validate() error {
	if strings.TrimSpace(tx.EndToEndID) == "" {
		return ErrEndToEndIDRequired
	}
	if len(tx.EndToEndID) > MaxText35 || len(tx.DebtorName) > MaxText140 || len(tx.Purpose) > MaxText140 {
		return ErrFieldTooLong
	}
	if tx.AmountCents <= 0 {
		return ErrAmountNotPositive
	}
	if strings.TrimSpace(tx.MandateID) == "" {
		return ErrMandateIDRequired
	}
	if len(tx.MandateID) > MaxText35 {
		return ErrFieldTooLong
	}
	if tx.MandateDate.IsZero() {
		return ErrMandateDateRequired
	}
	if strings.TrimSpace(tx.DebtorName) == "" {
		return ErrDebtorNameRequired
	}
	if !ValidIBAN(tx.DebtorIBAN) {
		return ErrDebtorIBANInvalid
	}
	if strings.TrimSpace(tx.DebtorBIC) == "" {
		return ErrDebtorAgentRequired
	}
	if !ValidBIC(tx.DebtorBIC) {
		return ErrDebtorBICInvalid
	}
	return nil
}

// Marshal renders the one-instruction message as pain.008.001.08 XML, or returns a validation error.
func (dd DirectDebit) Marshal() (string, error) {
	return dd.Batch().Marshal()
}

// Marshal renders the message as pain.008.001.08 XML, or returns a validation error. The output is
// deterministic for a given input, so it can be pinned by a golden file.
func (b DirectDebitBatch) Marshal() (string, error) {
	if err := b.validate(); err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	out.WriteString(`<Document xmlns="` + Pain008Namespace +
		`" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="` +
		Pain008SchemaLocation + `">` + "\n")
	out.WriteString("  <CstmrDrctDbtInitn>\n")
	b.writeGroupHeader(&out)
	for _, g := range b.Groups {
		b.writePaymentInstruction(&out, g)
	}
	out.WriteString("  </CstmrDrctDbtInitn>\n")
	out.WriteString("</Document>\n")
	return out.String(), nil
}

func (b DirectDebitBatch) writeGroupHeader(out *strings.Builder) {
	out.WriteString("    <GrpHdr>\n")
	textElem(out, 3, "MsgId", b.MessageID)
	textElem(out, 3, "CreDtTm", b.CreationTime.Format("2006-01-02T15:04:05"))
	textElem(out, 3, "NbOfTxs", strconv.Itoa(b.Count()))
	textElem(out, 3, "CtrlSum", FormatAmount(b.Sum()))
	out.WriteString("      <InitgPty>\n")
	textElem(out, 4, "Nm", b.InitiatingParty)
	out.WriteString("      </InitgPty>\n")
	out.WriteString("    </GrpHdr>\n")
}

// writePaymentInstruction writes one PmtInf. The sequence type and the collection date belong to the
// instruction, so a batch of several types repeats this block once per type.
func (b DirectDebitBatch) writePaymentInstruction(out *strings.Builder, g DirectDebitGroup) {
	out.WriteString("    <PmtInf>\n")
	textElem(out, 3, "PmtInfId", g.InstructionID)
	textElem(out, 3, "PmtMtd", "DD")
	textElem(out, 3, "NbOfTxs", strconv.Itoa(len(g.Transactions)))
	textElem(out, 3, "CtrlSum", FormatAmount(g.sum()))
	out.WriteString("      <PmtTpInf>\n")
	out.WriteString("        <SvcLvl>\n")
	textElem(out, 5, "Cd", "SEPA")
	out.WriteString("        </SvcLvl>\n")
	out.WriteString("        <LclInstrm>\n")
	textElem(out, 5, "Cd", b.LocalInstrument)
	out.WriteString("        </LclInstrm>\n")
	textElem(out, 4, "SeqTp", g.SequenceType)
	out.WriteString("      </PmtTpInf>\n")
	textElem(out, 3, "ReqdColltnDt", g.CollectionDate.Format("2006-01-02"))
	out.WriteString("      <Cdtr>\n")
	textElem(out, 4, "Nm", b.CreditorName)
	out.WriteString("      </Cdtr>\n")
	out.WriteString("      <CdtrAcct>\n        <Id>\n")
	textElem(out, 5, "IBAN", b.CreditorIBAN)
	out.WriteString("        </Id>\n      </CdtrAcct>\n")
	if b.CreditorBIC != "" {
		out.WriteString("      <CdtrAgt>\n        <FinInstnId>\n")
		textElem(out, 5, "BICFI", b.CreditorBIC)
		out.WriteString("        </FinInstnId>\n      </CdtrAgt>\n")
	}
	textElem(out, 3, "ChrgBr", "SLEV")
	// The SEPA creditor identifier sits in CdtrSchmeId/Id/PrvtId/Othr.
	out.WriteString("      <CdtrSchmeId>\n        <Id>\n          <PrvtId>\n            <Othr>\n")
	textElem(out, 7, "Id", b.CreditorID)
	out.WriteString("              <SchmeNm>\n")
	// The scheme name in the creditor identifier is proprietary by definition; the
	// GBIC profile rejects <Cd>SEPA</Cd> here (expects Prtry).
	textElem(out, 8, "Prtry", "SEPA")
	out.WriteString("              </SchmeNm>\n")
	out.WriteString("            </Othr>\n          </PrvtId>\n        </Id>\n      </CdtrSchmeId>\n")
	for _, tx := range g.Transactions {
		b.writeTransaction(out, tx)
	}
	out.WriteString("    </PmtInf>\n")
}

func (b DirectDebitBatch) writeTransaction(out *strings.Builder, tx DirectDebitTransaction) {
	out.WriteString("      <DrctDbtTxInf>\n")
	out.WriteString("        <PmtId>\n")
	textElem(out, 5, "EndToEndId", tx.EndToEndID)
	out.WriteString("        </PmtId>\n")
	out.WriteString(`        <InstdAmt Ccy="` + b.currency() + `">` + FormatAmount(tx.AmountCents) + "</InstdAmt>\n")
	out.WriteString("        <DrctDbtTx>\n          <MndtRltdInf>\n")
	textElem(out, 6, "MndtId", tx.MandateID)
	textElem(out, 6, "DtOfSgntr", tx.MandateDate.Format("2006-01-02"))
	out.WriteString("          </MndtRltdInf>\n        </DrctDbtTx>\n")
	// DbtrAgt is mandatory (1..1) in pain.008 and must precede Dbtr.
	out.WriteString("        <DbtrAgt>\n          <FinInstnId>\n")
	textElem(out, 6, "BICFI", tx.DebtorBIC)
	out.WriteString("          </FinInstnId>\n        </DbtrAgt>\n")
	out.WriteString("        <Dbtr>\n")
	textElem(out, 5, "Nm", tx.DebtorName)
	out.WriteString("        </Dbtr>\n")
	out.WriteString("        <DbtrAcct>\n          <Id>\n")
	textElem(out, 6, "IBAN", tx.DebtorIBAN)
	out.WriteString("          </Id>\n        </DbtrAcct>\n")
	out.WriteString("        <RmtInf>\n")
	textElem(out, 5, "Ustrd", tx.Purpose)
	out.WriteString("        </RmtInf>\n")
	out.WriteString("      </DrctDbtTxInf>\n")
}
