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

// validate checks every constraint the XSD and the DK profile impose.
func (dd DirectDebit) validate() error {
	if strings.TrimSpace(dd.MessageID) == "" {
		return ErrMessageIDRequired
	}
	if len(dd.MessageID) > MaxText35 {
		return ErrFieldTooLong
	}
	if dd.CreationTime.IsZero() {
		return ErrCreationTimeRequired
	}
	if strings.TrimSpace(dd.InitiatingParty) == "" {
		return ErrInitiatingPartyRequired
	}
	if len(dd.InitiatingParty) > MaxText140 || len(dd.CreditorName) > MaxText140 {
		return ErrFieldTooLong
	}
	if strings.TrimSpace(dd.CreditorName) == "" {
		return ErrCreditorNameRequired
	}
	if !ValidIBAN(dd.CreditorIBAN) {
		return ErrCreditorIBANInvalid
	}
	if dd.CreditorBIC != "" && !ValidBIC(dd.CreditorBIC) {
		return ErrCreditorBICInvalid
	}
	if strings.TrimSpace(dd.CreditorID) == "" {
		return ErrCreditorIDRequired
	}
	if len(dd.CreditorID) > MaxText35 {
		return ErrFieldTooLong
	}
	if !validSequenceType(dd.SequenceType) {
		return ErrSequenceTypeInvalid
	}
	if !validLocalInstrument(dd.LocalInstrument) {
		return ErrLocalInstrumentInvalid
	}
	if !validCurrency(dd.currency()) {
		return ErrCurrencyInvalid
	}
	if dd.CollectionDate.IsZero() {
		return ErrCollectionDateRequired
	}
	if len(dd.Transactions) == 0 {
		return ErrNoTransactions
	}
	for _, tx := range dd.Transactions {
		if err := tx.validate(); err != nil {
			return err
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
	if tx.DebtorBIC != "" && !ValidBIC(tx.DebtorBIC) {
		return ErrDebtorBICInvalid
	}
	return nil
}

// Marshal renders the message as pain.008.001.08 XML, or returns a validation
// error. The output is deterministic for a given input, so it can be pinned by a
// golden file.
func (dd DirectDebit) Marshal() (string, error) {
	if err := dd.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<Document xmlns="` + Pain008Namespace +
		`" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="` +
		Pain008SchemaLocation + `">` + "\n")
	b.WriteString("  <CstmrDrctDbtInitn>\n")
	dd.writeGroupHeader(&b)
	dd.writePaymentInstruction(&b)
	b.WriteString("  </CstmrDrctDbtInitn>\n")
	b.WriteString("</Document>\n")
	return b.String(), nil
}

func (dd DirectDebit) writeGroupHeader(b *strings.Builder) {
	b.WriteString("    <GrpHdr>\n")
	textElem(b, 3, "MsgId", dd.MessageID)
	textElem(b, 3, "CreDtTm", dd.CreationTime.Format("2006-01-02T15:04:05"))
	textElem(b, 3, "NbOfTxs", strconv.Itoa(len(dd.Transactions)))
	textElem(b, 3, "CtrlSum", FormatAmount(dd.Sum()))
	b.WriteString("      <InitgPty>\n")
	textElem(b, 4, "Nm", dd.InitiatingParty)
	b.WriteString("      </InitgPty>\n")
	b.WriteString("    </GrpHdr>\n")
}

func (dd DirectDebit) writePaymentInstruction(b *strings.Builder) {
	b.WriteString("    <PmtInf>\n")
	textElem(b, 3, "PmtInfId", dd.MessageID)
	textElem(b, 3, "PmtMtd", "DD")
	textElem(b, 3, "NbOfTxs", strconv.Itoa(len(dd.Transactions)))
	textElem(b, 3, "CtrlSum", FormatAmount(dd.Sum()))
	b.WriteString("      <PmtTpInf>\n")
	b.WriteString("        <SvcLvl>\n")
	textElem(b, 5, "Cd", "SEPA")
	b.WriteString("        </SvcLvl>\n")
	b.WriteString("        <LclInstrm>\n")
	textElem(b, 5, "Cd", dd.LocalInstrument)
	b.WriteString("        </LclInstrm>\n")
	textElem(b, 4, "SeqTp", dd.SequenceType)
	b.WriteString("      </PmtTpInf>\n")
	textElem(b, 3, "ReqdColltnDt", dd.CollectionDate.Format("2006-01-02"))
	b.WriteString("      <Cdtr>\n")
	textElem(b, 4, "Nm", dd.CreditorName)
	b.WriteString("      </Cdtr>\n")
	b.WriteString("      <CdtrAcct>\n        <Id>\n")
	textElem(b, 5, "IBAN", dd.CreditorIBAN)
	b.WriteString("        </Id>\n      </CdtrAcct>\n")
	if dd.CreditorBIC != "" {
		b.WriteString("      <CdtrAgt>\n        <FinInstnId>\n")
		textElem(b, 5, "BICFI", dd.CreditorBIC)
		b.WriteString("        </FinInstnId>\n      </CdtrAgt>\n")
	}
	textElem(b, 3, "ChrgBr", "SLEV")
	// The SEPA creditor identifier sits in CdtrSchmeId/Id/PrvtId/Othr.
	b.WriteString("      <CdtrSchmeId>\n        <Id>\n          <PrvtId>\n            <Othr>\n")
	textElem(b, 7, "Id", dd.CreditorID)
	b.WriteString("              <SchmeNm>\n")
	textElem(b, 8, "Cd", "SEPA")
	b.WriteString("              </SchmeNm>\n")
	b.WriteString("            </Othr>\n          </PrvtId>\n        </Id>\n      </CdtrSchmeId>\n")
	for _, tx := range dd.Transactions {
		dd.writeTransaction(b, tx)
	}
	b.WriteString("    </PmtInf>\n")
}

func (dd DirectDebit) writeTransaction(b *strings.Builder, tx DirectDebitTransaction) {
	b.WriteString("      <DrctDbtTxInf>\n")
	b.WriteString("        <PmtId>\n")
	textElem(b, 5, "EndToEndId", tx.EndToEndID)
	b.WriteString("        </PmtId>\n")
	b.WriteString(`        <InstdAmt Ccy="` + dd.currency() + `">` + FormatAmount(tx.AmountCents) + "</InstdAmt>\n")
	b.WriteString("        <DrctDbtTx>\n          <MndtRltdInf>\n")
	textElem(b, 6, "MndtId", tx.MandateID)
	textElem(b, 6, "DtOfSgntr", tx.MandateDate.Format("2006-01-02"))
	b.WriteString("          </MndtRltdInf>\n        </DrctDbtTx>\n")
	if tx.DebtorBIC != "" {
		b.WriteString("        <DbtrAgt>\n          <FinInstnId>\n")
		textElem(b, 6, "BICFI", tx.DebtorBIC)
		b.WriteString("          </FinInstnId>\n        </DbtrAgt>\n")
	}
	b.WriteString("        <Dbtr>\n")
	textElem(b, 5, "Nm", tx.DebtorName)
	b.WriteString("        </Dbtr>\n")
	b.WriteString("        <DbtrAcct>\n          <Id>\n")
	textElem(b, 6, "IBAN", tx.DebtorIBAN)
	b.WriteString("          </Id>\n        </DbtrAcct>\n")
	b.WriteString("        <RmtInf>\n")
	textElem(b, 5, "Ustrd", tx.Purpose)
	b.WriteString("        </RmtInf>\n")
	b.WriteString("      </DrctDbtTxInf>\n")
}
