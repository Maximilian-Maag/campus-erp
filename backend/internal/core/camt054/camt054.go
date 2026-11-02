// Package camt054 parses ISO 20022 camt.054 debit/credit notifications (XML).
//
// A camt.054 carries the same entry set as the matching statement, but the message
// type has no opening/closing balance, so the parsed statements carry lines only.
// It produces the shared bankstmt shape, so CAMT.054 feeds the same reconciliation
// as MT940 and CAMT.053. The field mapping follows the reference implementation parser
// (the reference parser.sepa.camt054v02/v08).
package camt054

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/bankstmt"
)

// Parse errors.
var (
	ErrInvalid        = errors.New("camt054: malformed document")
	ErrNoNotification = errors.New("camt054: no notification in document")
)

// document mirrors the camt.054 Document/BkToCstmrDbtCdtNtfctn structure.
type document struct {
	Ntfctns []ntfctn `xml:"BkToCstmrDbtCdtNtfctn>Ntfctn"`
}

type ntfctn struct {
	ID    string `xml:"Id"`
	Acct  acct   `xml:"Acct"`
	Ntrys []ntry `xml:"Ntry"`
}

type acct struct {
	ID  acctID `xml:"Id"`
	Ccy string `xml:"Ccy"`
}

type acctID struct {
	IBAN string `xml:"IBAN"`
}

type amount struct {
	Ccy   string `xml:"Ccy,attr"`
	Value string `xml:",chardata"`
}

type dateChoice struct {
	Dt string `xml:"Dt"`
}

type ntry struct {
	Amt       amount     `xml:"Amt"`
	CdtDbtInd string     `xml:"CdtDbtInd"`
	BookgDt   dateChoice `xml:"BookgDt"`
	ValDt     dateChoice `xml:"ValDt"`
	NtryDtls  ntryDtls   `xml:"NtryDtls"`
}

type ntryDtls struct {
	TxDtls []txDtls `xml:"TxDtls"`
}

type txDtls struct {
	Refs      refs      `xml:"Refs"`
	RltdPties rltdPties `xml:"RltdPties"`
	RmtInf    rmtInf    `xml:"RmtInf"`
}

type refs struct {
	EndToEndID string `xml:"EndToEndId"`
	TxID       string `xml:"TxId"`
}

// rltdPties mirrors TransactionParties6 (ISO 20022). Each party is a
// Party40Choice: the name sits under Pty (a party may instead be an Agt, which we
// do not read), and the account is a sibling element (DbtrAcct/CdtrAcct) rather
// than nested inside the party.
type rltdPties struct {
	Cdtr     party40Choice `xml:"Cdtr"`
	Dbtr     party40Choice `xml:"Dbtr"`
	CdtrAcct partyAcct     `xml:"CdtrAcct"`
	DbtrAcct partyAcct     `xml:"DbtrAcct"`
}

type party40Choice struct {
	Pty partyID `xml:"Pty"`
}

type partyID struct {
	Nm string `xml:"Nm"`
}

type partyAcct struct {
	ID acctID `xml:"Id"`
}

type rmtInf struct {
	Ustrd []string `xml:"Ustrd"`
}

// Parse parses a camt.054 document into the shared statement shape. Balances are
// left zero: a notification carries none.
func Parse(text string) ([]bankstmt.Statement, error) {
	var doc document
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(doc.Ntfctns) == 0 {
		return nil, ErrNoNotification
	}
	out := make([]bankstmt.Statement, 0, len(doc.Ntfctns))
	for _, n := range doc.Ntfctns {
		st := bankstmt.Statement{
			Reference: n.ID,
			Account:   n.Acct.ID.IBAN,
			Page:      1,
			Currency:  n.Acct.Ccy,
		}
		for _, e := range n.Ntrys {
			st.Lines = append(st.Lines, parseEntry(e))
		}
		out = append(out, st)
	}
	return out, nil
}

func parseEntry(n ntry) bankstmt.Line {
	credit := n.CdtDbtInd != "DBIT"
	line := bankstmt.Line{
		ValueDate:   parseDate(n.ValDt.Dt),
		BookingDate: parseDate(n.BookgDt.Dt),
		Credit:      credit,
		AmountCents: parseAmount(n.Amt.Value),
		Currency:    n.Amt.Ccy,
	}
	if len(n.NtryDtls.TxDtls) == 0 {
		return line
	}
	tx := n.NtryDtls.TxDtls[0]
	line.EndToEndRef = strings.TrimSpace(tx.Refs.EndToEndID)
	line.ClientRef = strings.TrimSpace(tx.Refs.TxID)
	line.Purpose = strings.TrimSpace(strings.Join(tx.RmtInf.Ustrd, " "))
	// The counterparty is the payer for a credit and the payee for a debit;
	// prefer whichever party the message carries.
	cp := tx.RltdPties.Cdtr.Pty
	acct := tx.RltdPties.CdtrAcct
	if strings.TrimSpace(cp.Nm) == "" {
		cp = tx.RltdPties.Dbtr.Pty
		acct = tx.RltdPties.DbtrAcct
	}
	line.CounterpartyName = strings.TrimSpace(cp.Nm)
	line.CounterpartyIBAN = strings.TrimSpace(acct.ID.IBAN)
	return line
}

// parseAmount parses a camt amount ("1250.00") into cents. A comma is treated
// as the decimal separator too, for tolerance.
func parseAmount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	intPart, fracPart := s, ""
	if i := strings.LastIndexAny(s, ".,"); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
	}
	intPart = strings.ReplaceAll(intPart, ",", "")
	fracPart = strings.ReplaceAll(fracPart, " ", "")
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 2 {
		fracPart = fracPart[:2]
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	units, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0
	}
	cents, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0
	}
	total := units*100 + cents
	if neg {
		return -total
	}
	return total
}

// parseDate parses an ISO date (YYYY-MM-DD) or a date-time; a malformed value
// yields the zero time.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if len(s) >= 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
