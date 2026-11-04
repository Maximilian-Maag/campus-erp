package kasse

import (
	"errors"
	"time"
)

// Slice S4: bank statement import (MT940) and reconciliation.
//
// A statement line is matched to the payment order that caused it, by the order
// reference carried in the line's client reference. A matched line whose
// direction is a return is classified as returned.

// Reconciliation outcomes for a statement line.
const (
	StatusUnmatched = "unmatched"
	StatusMatched   = "matched"
	StatusReturned  = "returned"
)

var (
	// ErrStatementEmpty is returned when the imported file has no statement.
	ErrStatementEmpty = errors.New("kasse: the statement file is empty")
	// ErrStatementNotFound is returned when a statement does not exist.
	ErrStatementNotFound = errors.New("kasse: bank statement not found")
	// ErrBankFileNotFound is returned when a bank file does not exist.
	ErrBankFileNotFound = errors.New("kasse: bank file not found")
	// ErrStatementInvalid is returned when the file cannot be parsed as MT940.
	ErrStatementInvalid = errors.New("kasse: malformed statement file")
)

// StatementLine is one booking with its reconciliation outcome.
type StatementLine struct {
	ID               int64
	LineNo           int
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
	UnclearedCents   int64
	Status           string
	OrderID          *int64
}

// BankFile is one imported file. A file may contain several statements.
type BankFile struct {
	ID         int64
	AccountID  int64
	FileName   string
	FileHash   string
	ImportedBy string
}

// StatementSummary is an imported statement as a list shows it: which account it belongs to, the
// period it covers with its opening and closing balance, and how many lines came with it.
type StatementSummary struct {
	ID                  int64
	AccountID           int64
	Reference           string
	Number              int
	Page                int
	Currency            string
	OpeningBalanceCents int64
	OpeningDate         time.Time
	ClosingBalanceCents int64
	ClosingDate         time.Time
	LineCount           int
	ImportedBy          string
}

// Statement is one imported bank statement with its lines.
type Statement struct {
	ID                  int64
	AccountID           int64
	BankFileID          int64
	Reference           string
	Number              int
	Page                int
	Currency            string
	OpeningBalanceCents int64
	OpeningDate         time.Time
	ClosingBalanceCents int64
	ClosingDate         time.Time
	LineCount           int
	ImportedBy          string
	Lines               []StatementLine
}
