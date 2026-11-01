// Package camt052 parses ISO 20022 camt.052 bank-to-customer account reports
// (intraday statements, XML).
//
// It produces the shared parsed-statement shape (bankstmt), so CAMT and MT940
// feed the same reconciliation. A report may carry balances; OPBD/CLBD are mapped
// when present and left zero otherwise. The field mapping follows the productive
// HISinOne camt.052 parser (rm.fia.payments.datex.sepa.camt052v02/v08).
package camt052

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
	ErrInvalid  = errors.New("camt052: malformed document")
	ErrNoReport = errors.New("camt052: no account report in document")
)

// document mirrors the camt.052 Document/BkToCstmrAcctRpt structure.
type document struct {
	Stmts []stmt `xml:"BkToCstmrAcctRpt>Rpt"`
}

type stmt struct {
	ID           string `xml:"Id"`
	ElctrncSeqNb int    `xml:"ElctrncSeqNb"`
	Acct         acct   `xml:"Acct"`
	Bals         []bal  `xml:"Bal"`
	Ntrys        []ntry `xml:"Ntry"`
}

type acct struct {
	ID  acctID `xml:"Id"`
	Ccy string `xml:"Ccy"`
}

type acctID struct {
	IBAN string `xml:"IBAN"`
}

type bal struct {
	Tp        balTp      `xml:"Tp"`
	Amt       amount     `xml:"Amt"`
	CdtDbtInd string     `xml:"CdtDbtInd"`
	Dt        dateChoice `xml:"Dt"`
}

type balTp struct {
	CdOrPrtry cdOrPrtry `xml:"CdOrPrtry"`
}

type cdOrPrtry struct {
	Cd string `xml:"Cd"`
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

type rltdPties struct {
	Cdtr party `xml:"Cdtr"`
	Dbtr party `xml:"Dbtr"`
}

type party struct {
	Nm   string    `xml:"Nm"`
	Acct partyAcct `xml:"Acct"`
}

type partyAcct struct {
	ID acctID `xml:"Id"`
}

type rmtInf struct {
	Ustrd []string `xml:"Ustrd"`
}

// Parse parses a camt.052 account report into the shared statement shape.
func Parse(text string) ([]bankstmt.Statement, error) {
	var doc document
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(doc.Stmts) == 0 {
		return nil, ErrNoReport
	}
	out := make([]bankstmt.Statement, 0, len(doc.Stmts))
	for _, s := range doc.Stmts {
		st := bankstmt.Statement{
			Reference: s.ID,
			Account:   s.Acct.ID.IBAN,
			Number:    s.ElctrncSeqNb,
			Page:      1,
			Currency:  s.Acct.Ccy,
		}
		for _, b := range s.Bals {
			cents := parseAmount(b.Amt.Value)
			if b.CdtDbtInd == "DBIT" {
				cents = -cents
			}
			switch b.Tp.CdOrPrtry.Cd {
			case "OPBD":
				st.OpeningBalanceCents = cents
				st.OpeningDate = parseDate(b.Dt.Dt)
			case "CLBD":
				st.ClosingBalanceCents = cents
				st.ClosingDate = parseDate(b.Dt.Dt)
			}
		}
		for _, n := range s.Ntrys {
			st.Lines = append(st.Lines, parseEntry(n))
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
	cp := tx.RltdPties.Cdtr
	if cp.Nm == "" {
		cp = tx.RltdPties.Dbtr
	}
	line.CounterpartyName = strings.TrimSpace(cp.Nm)
	line.CounterpartyIBAN = strings.TrimSpace(cp.Acct.ID.IBAN)
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
