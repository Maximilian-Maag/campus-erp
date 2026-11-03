package einvoice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

type fakeRepo struct {
	inv core.Invoice
	err error
}

func (f *fakeRepo) Insert(context.Context, core.Invoice) (core.Invoice, error) {
	return f.inv, f.err
}

func (f *fakeRepo) Get(context.Context, int64) (core.Invoice, error) {
	return f.inv, f.err
}

type fakeMatcher struct {
	calls     int
	gotNumber string
	gotGross  int64
	gotRef    string
	gotActor  string
	order     procurement.Order
	err       error
}

func (f *fakeMatcher) MatchReceipt(_ context.Context, orderNumber string, grossCents int64, reference, by string) (procurement.Order, error) {
	f.calls++
	f.gotNumber, f.gotGross, f.gotRef, f.gotActor = orderNumber, grossCents, reference, by
	return f.order, f.err
}

type fakeAudit struct {
	events []audit.Event
	err    error
}

func (f *fakeAudit) Append(_ context.Context, e audit.Event) (audit.Event, error) {
	if f.err != nil {
		return audit.Event{}, f.err
	}
	f.events = append(f.events, e)
	return e, nil
}

func matchedInvoice() core.Invoice {
	return core.Invoice{
		ID: 7, Number: "RE-1", BuyerReference: "PO-2026-1",
		IssueDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		TypeCode:  "380", Currency: "EUR",
		Lines: []core.Line{{LineNo: 1, Description: "D", QuantityMilli: 1000, UnitCode: "C62",
			UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}},
	}
}

// TestMatchDelegatesWithTheBuyerReference: the match keys on the buyer reference the document
// carries (BT-10), passes the invoice's own gross on, and audits the match.
func TestMatchDelegatesWithTheBuyerReference(t *testing.T) {
	matcher := &fakeMatcher{order: procurement.Order{ID: 3, Number: "PO-2026-1"}}
	sink := &fakeAudit{}
	svc := NewInvoiceService(&fakeRepo{inv: matchedInvoice()}, matcher, sink)

	got, err := svc.Match(context.Background(), 7, "carol")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.ID != 7 {
		t.Fatalf("Match returned invoice %d, want the matched one", got.ID)
	}
	if matcher.calls != 1 || matcher.gotNumber != "PO-2026-1" || matcher.gotActor != "carol" {
		t.Fatalf("matcher got (%d, %q, %q)", matcher.calls, matcher.gotNumber, matcher.gotActor)
	}
	if matcher.gotGross != 35700 { // 30000 net + 19 % VAT
		t.Fatalf("matcher got gross %d, want the invoice's 35700", matcher.gotGross)
	}
	if matcher.gotRef != "RE:RE-1" {
		t.Fatalf("matcher got reference %q, want it named after the invoice", matcher.gotRef)
	}
	if len(sink.events) != 1 || sink.events[0].Action != "einvoice.match" {
		t.Fatalf("audit = %+v, want one einvoice.match", sink.events)
	}
}

// TestMatchRefusals: everything that must stop a match before it touches the order.
func TestMatchRefusals(t *testing.T) {
	boom := errors.New("boom")

	cases := []struct {
		name    string
		svc     *InvoiceService
		actor   string
		wantErr error
	}{
		{"no actor", NewInvoiceService(&fakeRepo{inv: matchedInvoice()}, &fakeMatcher{}, &fakeAudit{}), "  ", ErrActorRequired},
		{"no matcher", NewInvoiceService(&fakeRepo{inv: matchedInvoice()}, nil, &fakeAudit{}), "carol", ErrNoOrderMatcher},
		{"repo fails", NewInvoiceService(&fakeRepo{err: boom}, &fakeMatcher{}, &fakeAudit{}), "carol", boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.svc.Match(context.Background(), 7, c.actor); !errors.Is(err, c.wantErr) {
				t.Fatalf("Match = %v, want %v", err, c.wantErr)
			}
		})
	}

	// An invoice with no buyer reference cannot name an order.
	noRef := matchedInvoice()
	noRef.BuyerReference = " "
	svc := NewInvoiceService(&fakeRepo{inv: noRef}, &fakeMatcher{}, &fakeAudit{})
	if _, err := svc.Match(context.Background(), 7, "carol"); !errors.Is(err, core.ErrBuyerReference) {
		t.Fatalf("Match without a buyer reference = %v, want core.ErrBuyerReference", err)
	}
}

// TestMatchPropagatesMatcherAndAuditFailures: a refused match (wrong amount, wrong state) and a
// failed audit both surface rather than returning a matched invoice.
func TestMatchPropagatesMatcherAndAuditFailures(t *testing.T) {
	refused := &fakeMatcher{err: procurement.ErrMatchAmountMismatch}
	svc := NewInvoiceService(&fakeRepo{inv: matchedInvoice()}, refused, &fakeAudit{})
	if _, err := svc.Match(context.Background(), 7, "carol"); !errors.Is(err, procurement.ErrMatchAmountMismatch) {
		t.Fatalf("Match = %v, want the matcher's refusal", err)
	}

	boom := errors.New("audit down")
	svc = NewInvoiceService(&fakeRepo{inv: matchedInvoice()}, &fakeMatcher{}, &fakeAudit{err: boom})
	if _, err := svc.Match(context.Background(), 7, "carol"); !errors.Is(err, boom) {
		t.Fatalf("Match = %v, want the audit failure", err)
	}
}
