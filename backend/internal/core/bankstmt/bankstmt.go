// Package bankstmt holds the neutral shape of a parsed bank statement, shared by
// the format parsers (MT940 and CAMT.053) so that reconciliation has one input.
package bankstmt

import "time"

// Line is one booking on a statement.
type Line struct {
	ValueDate        time.Time
	BookingDate      time.Time
	Credit           bool
	Return           bool
	AmountCents      int64
	Currency         string
	PostingKey       string
	ClientRef        string
	BankRef          string
	Purpose          string
	EndToEndRef      string
	CounterpartyName string
	CounterpartyIBAN string
	CounterpartyBIC  string
}

// Statement is one parsed statement (one account, one statement).
type Statement struct {
	Reference           string
	Account             string
	Number              int
	Page                int
	Currency            string
	OpeningBalanceCents int64
	OpeningDate         time.Time
	ClosingBalanceCents int64
	ClosingDate         time.Time
	Lines               []Line
}
