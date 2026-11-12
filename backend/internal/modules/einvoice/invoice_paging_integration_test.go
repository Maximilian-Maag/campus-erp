package einvoice_test

import (
	"context"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
)

// TestInvoiceListPages drives the list the way a screen does: one page at a time, advancing the
// offset, and checks the two properties a paged list must have — a page holds no more than was
// asked for, and walking the pages yields exactly the whole list, in order, with nothing repeated
// and nothing skipped.
//
// It reads the list rather than assuming it is empty: the table is shared with the other tests in
// this package, and a list that pages correctly has to page correctly with whatever is in it.
func TestInvoiceListPages(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, sampleInvoice(t), "alice"); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// A page big enough for everything: the reference order the pages must reproduce.
	whole, err := svc.Invoices(ctx, 1000, 0)
	if err != nil {
		t.Fatalf("Invoices: %v", err)
	}
	if len(whole) < 3 {
		t.Fatalf("the list holds %d invoices, want at least the 3 just created", len(whole))
	}

	const pageSize = 2
	var walked []einvoice.InvoiceSummary
	for offset := 0; offset < len(whole); offset += pageSize {
		page, err := svc.Invoices(ctx, pageSize, offset)
		if err != nil {
			t.Fatalf("Invoices(offset=%d): %v", offset, err)
		}
		if len(page) > pageSize {
			t.Fatalf("page at offset %d holds %d rows, want at most %d", offset, len(page), pageSize)
		}
		walked = append(walked, page...)
	}

	if len(walked) != len(whole) {
		t.Fatalf("walking the pages yielded %d invoices, want %d", len(walked), len(whole))
	}
	for i := range whole {
		if walked[i].ID != whole[i].ID {
			t.Fatalf("page walk diverges at %d: got %d, want %d — the order is not stable", i, walked[i].ID, whole[i].ID)
		}
	}

	// Past the end is an empty page, not an error: a screen that asks once too often gets nothing
	// rather than a failure it would have to explain.
	past, err := svc.Invoices(ctx, pageSize, len(whole)+pageSize)
	if err != nil {
		t.Fatalf("Invoices past the end: %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows, want none", len(past))
	}
}
