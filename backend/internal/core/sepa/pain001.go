package sepa

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// This file generates a SEPA credit-transfer initiation, ISO 20022
// pain.001.001.09, in the DK/GBIC-compatible shape the reference implementation
// writer produces (the reference parser.parser.sepa.SEPAWriterCreditTransferVersion39).
//
// The element order follows the XSD sequences exactly:
//   GroupHeader85:      MsgId, CreDtTm, NbOfTxs, CtrlSum, InitgPty
//   PaymentInstruction30: PmtInfId, PmtMtd, BtchBookg, NbOfTxs, CtrlSum,
//                         PmtTpInf, ReqdExctnDt, Dbtr, DbtrAcct, DbtrAgt,
//                         ChrgBr, CdtTrfTxInf+
//   CreditTransferTransaction34: PmtId, Amt, CdtrAgt, Cdtr, CdtrAcct, RmtInf

const (
	// Pain001Namespace is the ISO 20022 namespace of pain.001.001.09.
	Pain001Namespace = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09"
	// Pain001SchemaLocation is the xsi:schemaLocation value the DK/GBIC profile
	// expects (note: two spaces, as emitted by the productive writer).
	Pain001SchemaLocation = "urn:iso:std:iso:20022:tech:xsd:pain.001.001.09  pain.001.001.09.xsd"

	// MaxText35 / MaxText140 are the ISO 20022 Max35Text / Max140Text limits.
	MaxText35  = 35
	MaxText140 = 140

	defaultCurrency = "EUR"
)

// Validation errors. They are values so callers can match them with errors.Is.
var (
	ErrMessageIDRequired       = errors.New("sepa: message id is required")
	ErrCreationTimeRequired    = errors.New("sepa: creation time is required")
	ErrInitiatingPartyRequired = errors.New("sepa: initiating party is required")
	ErrDebtorNameRequired      = errors.New("sepa: debtor name is required")
	ErrDebtorIBANInvalid       = errors.New("sepa: debtor IBAN is invalid")
	ErrDebtorBICInvalid        = errors.New("sepa: debtor BIC is invalid")
	ErrCurrencyInvalid         = errors.New("sepa: currency must be a three-letter ISO-4217 code")
	ErrExecutionDateRequired   = errors.New("sepa: execution date is required")
	ErrNoTransactions          = errors.New("sepa: at least one transaction is required")
	ErrEndToEndIDRequired      = errors.New("sepa: end-to-end id is required")
	ErrAmountNotPositive       = errors.New("sepa: transaction amount must be positive")
	ErrCreditorNameRequired    = errors.New("sepa: creditor name is required")
	ErrCreditorIBANInvalid     = errors.New("sepa: creditor IBAN is invalid")
	ErrCreditorBICInvalid      = errors.New("sepa: creditor BIC is invalid")
	ErrFieldTooLong            = errors.New("sepa: field exceeds the ISO 20022 length limit")
)

// Transaction is one credit transfer (one payment to one creditor).
type Transaction struct {
	EndToEndID   string
	AmountCents  int64
	CreditorName string
	CreditorIBAN string
	CreditorBIC  string
	Purpose      string
}

// CreditTransfer is a whole pain.001 message: one debtor account paying many
// creditors on one execution date.
type CreditTransfer struct {
	MessageID       string
	CreationTime    time.Time
	InitiatingParty string
	DebtorName      string
	DebtorIBAN      string
	DebtorBIC       string
	Currency        string
	ExecutionDate   time.Time
	Transactions    []Transaction
}

// currency returns the effective currency, defaulting to EUR.
func (ct CreditTransfer) currency() string {
	if ct.Currency == "" {
		return defaultCurrency
	}
	return ct.Currency
}

// Sum returns the total amount in cents.
func (ct CreditTransfer) Sum() int64 {
	var sum int64
	for _, tx := range ct.Transactions {
		sum += tx.AmountCents
	}
	return sum
}

// validate checks every constraint the XSD and the DK profile impose.
func (ct CreditTransfer) validate() error {
	if strings.TrimSpace(ct.MessageID) == "" {
		return ErrMessageIDRequired
	}
	if len(ct.MessageID) > MaxText35 {
		return ErrFieldTooLong
	}
	if ct.CreationTime.IsZero() {
		return ErrCreationTimeRequired
	}
	if strings.TrimSpace(ct.InitiatingParty) == "" {
		return ErrInitiatingPartyRequired
	}
	if len(ct.InitiatingParty) > MaxText140 || len(ct.DebtorName) > MaxText140 {
		return ErrFieldTooLong
	}
	if strings.TrimSpace(ct.DebtorName) == "" {
		return ErrDebtorNameRequired
	}
	if !ValidIBAN(ct.DebtorIBAN) {
		return ErrDebtorIBANInvalid
	}
	if ct.DebtorBIC != "" && !ValidBIC(ct.DebtorBIC) {
		return ErrDebtorBICInvalid
	}
	if !validCurrency(ct.currency()) {
		return ErrCurrencyInvalid
	}
	if ct.ExecutionDate.IsZero() {
		return ErrExecutionDateRequired
	}
	if len(ct.Transactions) == 0 {
		return ErrNoTransactions
	}
	for _, tx := range ct.Transactions {
		if err := tx.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (tx Transaction) validate() error {
	if strings.TrimSpace(tx.EndToEndID) == "" {
		return ErrEndToEndIDRequired
	}
	if len(tx.EndToEndID) > MaxText35 || len(tx.CreditorName) > MaxText140 || len(tx.Purpose) > MaxText140 {
		return ErrFieldTooLong
	}
	if tx.AmountCents <= 0 {
		return ErrAmountNotPositive
	}
	if strings.TrimSpace(tx.CreditorName) == "" {
		return ErrCreditorNameRequired
	}
	if !ValidIBAN(tx.CreditorIBAN) {
		return ErrCreditorIBANInvalid
	}
	if tx.CreditorBIC != "" && !ValidBIC(tx.CreditorBIC) {
		return ErrCreditorBICInvalid
	}
	return nil
}

// ValidBIC reports whether s is a syntactically valid ISO 9362 BIC: six letters,
// two alphanumerics, and an optional three-character branch code.
func ValidBIC(s string) bool {
	s = strings.ToUpper(s)
	if len(s) != 8 && len(s) != 11 {
		return false
	}
	for i := 0; i < 6; i++ {
		if !isLetter(rune(s[i])) {
			return false
		}
	}
	for i := 6; i < len(s); i++ {
		c := rune(s[i])
		if !isLetter(c) && !isDigit(c) {
			return false
		}
	}
	return true
}

func validCurrency(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if !isLetter(rune(s[i])) {
			return false
		}
	}
	return true
}

// Marshal renders the message as pain.001.001.09 XML, or returns a validation
// error. The output is deterministic for a given input, so it can be pinned by
// a golden file.
func (ct CreditTransfer) Marshal() (string, error) {
	if err := ct.validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<Document xmlns="` + Pain001Namespace +
		`" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="` +
		Pain001SchemaLocation + `">` + "\n")
	b.WriteString("  <CstmrCdtTrfInitn>\n")
	ct.writeGroupHeader(&b)
	ct.writePaymentInstruction(&b)
	b.WriteString("  </CstmrCdtTrfInitn>\n")
	b.WriteString("</Document>\n")
	return b.String(), nil
}

func (ct CreditTransfer) writeGroupHeader(b *strings.Builder) {
	b.WriteString("    <GrpHdr>\n")
	textElem(b, 3, "MsgId", ct.MessageID)
	textElem(b, 3, "CreDtTm", ct.CreationTime.Format("2006-01-02T15:04:05"))
	textElem(b, 3, "NbOfTxs", strconv.Itoa(len(ct.Transactions)))
	textElem(b, 3, "CtrlSum", FormatAmount(ct.Sum()))
	b.WriteString("      <InitgPty>\n")
	textElem(b, 4, "Nm", ct.InitiatingParty)
	b.WriteString("      </InitgPty>\n")
	b.WriteString("    </GrpHdr>\n")
}

func (ct CreditTransfer) writePaymentInstruction(b *strings.Builder) {
	b.WriteString("    <PmtInf>\n")
	textElem(b, 3, "PmtInfId", ct.MessageID)
	textElem(b, 3, "PmtMtd", "TRF")
	textElem(b, 3, "BtchBookg", "true")
	textElem(b, 3, "NbOfTxs", strconv.Itoa(len(ct.Transactions)))
	textElem(b, 3, "CtrlSum", FormatAmount(ct.Sum()))
	b.WriteString("      <PmtTpInf>\n        <SvcLvl>\n")
	textElem(b, 5, "Cd", "SEPA")
	b.WriteString("        </SvcLvl>\n      </PmtTpInf>\n")
	b.WriteString("      <ReqdExctnDt>\n")
	textElem(b, 4, "Dt", ct.ExecutionDate.Format("2006-01-02"))
	b.WriteString("      </ReqdExctnDt>\n")
	b.WriteString("      <Dbtr>\n")
	textElem(b, 4, "Nm", ct.DebtorName)
	b.WriteString("      </Dbtr>\n")
	b.WriteString("      <DbtrAcct>\n        <Id>\n")
	textElem(b, 5, "IBAN", ct.DebtorIBAN)
	b.WriteString("        </Id>\n")
	textElem(b, 4, "Ccy", ct.currency())
	b.WriteString("      </DbtrAcct>\n")
	if ct.DebtorBIC != "" {
		b.WriteString("      <DbtrAgt>\n        <FinInstnId>\n")
		textElem(b, 5, "BICFI", ct.DebtorBIC)
		b.WriteString("        </FinInstnId>\n      </DbtrAgt>\n")
	}
	textElem(b, 3, "ChrgBr", "SLEV")
	for _, tx := range ct.Transactions {
		ct.writeTransaction(b, tx)
	}
	b.WriteString("    </PmtInf>\n")
}

func (ct CreditTransfer) writeTransaction(b *strings.Builder, tx Transaction) {
	b.WriteString("      <CdtTrfTxInf>\n")
	b.WriteString("        <PmtId>\n")
	textElem(b, 5, "EndToEndId", tx.EndToEndID)
	b.WriteString("        </PmtId>\n")
	b.WriteString("        <Amt>\n")
	b.WriteString(`          <InstdAmt Ccy="` + ct.currency() + `">` + FormatAmount(tx.AmountCents) + "</InstdAmt>\n")
	b.WriteString("        </Amt>\n")
	if tx.CreditorBIC != "" {
		b.WriteString("        <CdtrAgt>\n          <FinInstnId>\n")
		textElem(b, 6, "BICFI", tx.CreditorBIC)
		b.WriteString("          </FinInstnId>\n        </CdtrAgt>\n")
	}
	b.WriteString("        <Cdtr>\n")
	textElem(b, 5, "Nm", tx.CreditorName)
	b.WriteString("        </Cdtr>\n")
	b.WriteString("        <CdtrAcct>\n          <Id>\n")
	textElem(b, 6, "IBAN", tx.CreditorIBAN)
	b.WriteString("          </Id>\n        </CdtrAcct>\n")
	b.WriteString("        <RmtInf>\n")
	textElem(b, 5, "Ustrd", tx.Purpose)
	b.WriteString("        </RmtInf>\n")
	b.WriteString("      </CdtTrfTxInf>\n")
}

// textElem writes "<name>escaped</name>" indented by depth levels of two spaces.
func textElem(b *strings.Builder, depth int, name, value string) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
	b.WriteByte('<')
	b.WriteString(name)
	b.WriteByte('>')
	b.WriteString(escapeXML(value))
	b.WriteString("</")
	b.WriteString(name)
	b.WriteString(">\n")
}

// escapeXML escapes the five XML metacharacters.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
