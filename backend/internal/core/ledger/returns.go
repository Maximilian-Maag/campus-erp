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

// The second document this package renders: the **return postings** a reconciled bank
// statement produces for the financial-ledger side (docs/30-return-postings.md).
//
// A returning line is money the bank took back — a direct debit the debtor's bank
// refused (posting key 009), a payment-out that came back (059). The cash movement is
// the *opposite* of the original, so its counterpart posting is the direction of the
// cash movement: money into our account debits cash and credits the counterpart, money
// out does the reverse. The file is versioned, deterministic and idempotent for the
// same reason the posting export is (docs/23): the target ledger re-imports by
// reference, so re-exporting a period must change nothing.

// ReturnsFormatVersion identifies the return-postings layout. It is written into the
// file, so a consumer can refuse a version it does not understand.
const ReturnsFormatVersion = "cash-returns/v1"

// Validation errors of the return export. They are values so callers can match them
// with errors.Is.
var (
	ErrReturnAccountRequired  = errors.New("ledger: the return account is required")
	ErrNoReturnPostings       = errors.New("ledger: a returns export needs at least one posting")
	ErrReturnDateRequired     = errors.New("ledger: a return posting needs a value date")
	ErrReturnPostingKeyNeeded = errors.New("ledger: a return posting needs the bank's posting key")
)

// ReturnPosting is one returning statement line expressed as a posting against the
// cash account — the counterpart of the movement the bank took back.
type ReturnPosting struct {
	// Reference is the idempotency key: it names the statement line, so re-exporting
	// the period carries the same reference and the ledger can de-duplicate.
	Reference string
	// ValueDate is the bank's value date; it is what the return is effective on, and
	// what the export is ordered by.
	ValueDate time.Time
	// BookingDate is the bank's booking date when the statement carried one.
	BookingDate time.Time
	// Account is the counterpart account the return posts against (cash itself is the
	// export's CashAccount).
	Account string
	// Direction is the direction of the *counterpart* posting, which is the direction
	// of the cash movement: a credit line (money in) debits cash and credits the
	// counterpart, a debit line does the reverse.
	Direction Direction
	// AmountCents is the returned amount in minor units; never a float.
	AmountCents int64
	// PostingKey is the bank's return posting key (`009` direct-debit return, `059`
	// payment-out return) — the evidence of what the bank did, kept in the file.
	PostingKey string
	// OrderReference is the order the returned movement belonged to, when the line
	// reconciled to one, so the ledger can tie the return to its original posting.
	OrderReference string
	Purpose        string
}

// ReturnExport is one period's return postings against one cash account.
type ReturnExport struct {
	From        time.Time
	To          time.Time
	CashAccount string
	Currency    string
	Postings    []ReturnPosting
}

func (e ReturnExport) currency() string {
	if strings.TrimSpace(e.Currency) == "" {
		return DefaultCurrency
	}
	return strings.ToUpper(strings.TrimSpace(e.Currency))
}

// Validate reports the first structural problem with the export or a posting.
func (e ReturnExport) Validate() error {
	if strings.TrimSpace(e.CashAccount) == "" {
		return ErrCashAccountRequired
	}
	if len(e.Postings) == 0 {
		return ErrNoReturnPostings
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

func (p ReturnPosting) validate() error {
	if strings.TrimSpace(p.Reference) == "" {
		return ErrReferenceRequired
	}
	if p.ValueDate.IsZero() {
		return ErrReturnDateRequired
	}
	if strings.TrimSpace(p.Account) == "" {
		return ErrReturnAccountRequired
	}
	if !p.Direction.Valid() {
		return ErrDirectionInvalid
	}
	if p.AmountCents <= 0 {
		return ErrAmountNotPositive
	}
	if strings.TrimSpace(p.PostingKey) == "" {
		return ErrReturnPostingKeyNeeded
	}
	return nil
}

// Marshal renders the return export as CSV, or returns a validation error. The
// postings are sorted by value date and then reference, so the same set always
// produces byte-identical output whatever order it was collected in.
func (e ReturnExport) Marshal() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	rows := make([]ReturnPosting, len(e.Postings))
	copy(rows, e.Postings)
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].ValueDate.Equal(rows[j].ValueDate) {
			return rows[i].ValueDate.Before(rows[j].ValueDate)
		}
		return rows[i].Reference < rows[j].Reference
	})

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", ReturnsFormatVersion)
	fmt.Fprintf(&b, "# period: %s..%s\n", e.From.Format("2006-01-02"), e.To.Format("2006-01-02"))
	fmt.Fprintf(&b, "# cash_account: %s\n", strings.TrimSpace(e.CashAccount))
	w := csv.NewWriter(&b)
	if err := w.Write([]string{
		"reference", "value_date", "booking_date", "account", "cash_account",
		"direction", "amount_cents", "currency", "posting_key", "order_reference", "purpose",
	}); err != nil {
		return "", fmt.Errorf("ledger: returns header: %w", err)
	}
	cash, ccy := strings.TrimSpace(e.CashAccount), e.currency()
	for _, p := range rows {
		booking := ""
		if !p.BookingDate.IsZero() {
			booking = p.BookingDate.Format("2006-01-02")
		}
		if err := w.Write([]string{
			strings.TrimSpace(p.Reference),
			p.ValueDate.Format("2006-01-02"),
			booking,
			strings.TrimSpace(p.Account),
			cash,
			string(p.Direction),
			strconv.FormatInt(p.AmountCents, 10),
			ccy,
			strings.TrimSpace(p.PostingKey),
			strings.TrimSpace(p.OrderReference),
			p.Purpose,
		}); err != nil {
			return "", fmt.Errorf("ledger: returns row %s: %w", p.Reference, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("ledger: returns: %w", err)
	}
	return b.String(), nil
}
