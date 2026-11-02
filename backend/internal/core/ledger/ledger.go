// Package ledger renders the posting export that carries confirmed cash bookings to
// the financial-ledger side.
//
// The format is defined by the target system (docs/23-ledger-postings.md): versioned,
// deterministic and ordered, so the same booking set always produces the same file and
// a consumer can re-import or reconcile without duplicate postings.
package ledger

import (
	"encoding/csv"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FormatVersion identifies the layout. It is written into the file, so a consumer can
// refuse a version it does not understand.
const FormatVersion = "cash-postings/v1"

// DefaultCurrency is used when an export does not name one.
const DefaultCurrency = "EUR"

// Validation errors. They are values so callers can match them with errors.Is.
var (
	ErrCashAccountRequired = errors.New("ledger: cash account is required")
	ErrNoPostings          = errors.New("ledger: an export needs at least one posting")
	ErrPeriodInvalid       = errors.New("ledger: period end precedes its start")
	ErrCurrencyInvalid     = errors.New("ledger: currency must be a three-letter code")
	ErrReferenceRequired   = errors.New("ledger: posting reference is required")
	ErrBookingDateRequired = errors.New("ledger: posting booking date is required")
	ErrAccountRequired     = errors.New("ledger: posting account is required")
	ErrDirectionInvalid    = errors.New("ledger: direction must be debit or credit")
	ErrAmountNotPositive   = errors.New("ledger: posting amount must be positive")
)

// Direction is the direction of the *counterpart* posting: an income credits the
// counterpart and debits cash, an expense does the opposite.
type Direction string

// The two posting directions.
const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// Valid reports whether d is a known direction.
func (d Direction) Valid() bool { return d == Debit || d == Credit }

// Posting is one confirmed booking expressed as a posting against the cash account.
type Posting struct {
	// Reference is the stable booking reference; re-exporting the same booking
	// carries the same reference, which is what makes the export idempotent.
	Reference   string
	BookingDate time.Time
	// Account is the counterpart account (cash itself is the export's CashAccount).
	Account     string
	Direction   Direction
	AmountCents int64
	Purpose     string
}

// Export is one period's postings against one cash account.
type Export struct {
	From        time.Time
	To          time.Time
	CashAccount string
	Currency    string
	Postings    []Posting
}

func (e Export) currency() string {
	if strings.TrimSpace(e.Currency) == "" {
		return DefaultCurrency
	}
	return strings.ToUpper(strings.TrimSpace(e.Currency))
}

// Validate reports the first structural problem with the export or a posting.
func (e Export) Validate() error {
	if strings.TrimSpace(e.CashAccount) == "" {
		return ErrCashAccountRequired
	}
	if len(e.Postings) == 0 {
		return ErrNoPostings
	}
	if !e.To.IsZero() && !e.From.IsZero() && e.To.Before(e.From) {
		return ErrPeriodInvalid
	}
	if c := e.currency(); len(c) != 3 {
		return ErrCurrencyInvalid
	}
	for _, p := range e.Postings {
		if err := p.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p Posting) validate() error {
	if strings.TrimSpace(p.Reference) == "" {
		return ErrReferenceRequired
	}
	if p.BookingDate.IsZero() {
		return ErrBookingDateRequired
	}
	if strings.TrimSpace(p.Account) == "" {
		return ErrAccountRequired
	}
	if !p.Direction.Valid() {
		return ErrDirectionInvalid
	}
	if p.AmountCents <= 0 {
		return ErrAmountNotPositive
	}
	return nil
}

// Marshal renders the export as CSV, or returns a validation error. The postings are
// sorted by booking date and then reference, so the output is deterministic for a
// given set regardless of the order it was collected in.
func (e Export) Marshal() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	rows := make([]Posting, len(e.Postings))
	copy(rows, e.Postings)
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].BookingDate.Equal(rows[j].BookingDate) {
			return rows[i].BookingDate.Before(rows[j].BookingDate)
		}
		return rows[i].Reference < rows[j].Reference
	})

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", FormatVersion)
	fmt.Fprintf(&b, "# period: %s..%s\n", e.From.Format("2006-01-02"), e.To.Format("2006-01-02"))
	fmt.Fprintf(&b, "# cash_account: %s\n", strings.TrimSpace(e.CashAccount))
	w := csv.NewWriter(&b)
	if err := w.Write([]string{
		"reference", "booking_date", "account", "cash_account",
		"direction", "amount_cents", "currency", "purpose",
	}); err != nil {
		return "", fmt.Errorf("ledger: header: %w", err)
	}
	cash, ccy := strings.TrimSpace(e.CashAccount), e.currency()
	for _, p := range rows {
		if err := w.Write([]string{
			strings.TrimSpace(p.Reference),
			p.BookingDate.Format("2006-01-02"),
			strings.TrimSpace(p.Account),
			cash,
			string(p.Direction),
			strconv.FormatInt(p.AmountCents, 10),
			ccy,
			p.Purpose,
		}); err != nil {
			return "", fmt.Errorf("ledger: row %s: %w", p.Reference, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("ledger: %w", err)
	}
	return b.String(), nil
}
