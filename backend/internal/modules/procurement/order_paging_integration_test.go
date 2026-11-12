package procurement_test

import (
	"context"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// TestOrderListPages drives the purchase-order list the way the screen does — one page at a time,
// advancing the offset — and checks the properties a paged, filtered list must have: a page holds no
// more than was asked for, walking the pages yields exactly the whole list in order (nothing
// repeated, nothing skipped), and the status filter holds across the pages rather than only on the
// first.
//
// It reads the list rather than assuming it is empty: the list can be filtered by status but not by
// order number, so it walks whatever the package's other tests created as well.
func TestOrderListPages(t *testing.T) {
	ctx := context.Background()
	procSvc, budgetSvc := setup(t)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "05", "L-Seiten-"+suffix(t), "Beschaffung", 1000000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	// Three approved orders and one draft: a page boundary that crossed the filter would show up as
	// an order of the wrong status rather than as a missing one.
	numbers := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		order, err := procSvc.Create(ctx, "PO-P"+marker(i)+"-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(1000+int64(i), 1900), "alice")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, err := procSvc.Approve(ctx, order.ID, "dave"); err != nil {
			t.Fatalf("Approve: %v", err)
		}
		numbers = append(numbers, order.Number)
	}
	if _, err := procSvc.Create(ctx, "PO-D1-"+suffix(t), "Lieferant GmbH", pos.ID, "EUR", oneLine(5000, 1900), "alice"); err != nil {
		t.Fatalf("Create draft: %v", err)
	}

	whole, err := procSvc.Orders(ctx, procurement.StatusApproved, 1000, 0)
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	if len(whole) < len(numbers) {
		t.Fatalf("the approved list holds %d orders, want at least the %d just approved", len(whole), len(numbers))
	}

	const pageSize = 2
	var walked []procurement.OrderSummary
	for offset := 0; offset < len(whole); offset += pageSize {
		page, err := procSvc.Orders(ctx, procurement.StatusApproved, pageSize, offset)
		if err != nil {
			t.Fatalf("Orders(offset=%d): %v", offset, err)
		}
		if len(page) > pageSize {
			t.Fatalf("page at offset %d holds %d rows, want at most %d", offset, len(page), pageSize)
		}
		for _, o := range page {
			if o.Status != procurement.StatusApproved {
				t.Fatalf("page at offset %d returned an order with status %q, not the filter's", offset, o.Status)
			}
		}
		walked = append(walked, page...)
	}
	if len(walked) != len(whole) {
		t.Fatalf("walking the pages yielded %d orders, want %d", len(walked), len(whole))
	}
	for i := range whole {
		if walked[i].ID != whole[i].ID {
			t.Fatalf("page walk diverges at %d: got %d, want %d — the order is not total", i, walked[i].ID, whole[i].ID)
		}
	}

	past, err := procSvc.Orders(ctx, procurement.StatusApproved, pageSize, len(whole)+pageSize)
	if err != nil {
		t.Fatalf("Orders past the end: %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows, want none", len(past))
	}

	// The draft filter is the other half: it returns drafts, and a page of the unfiltered list is a
	// page of *that* list.
	drafts, err := procSvc.Orders(ctx, procurement.StatusDraft, 1000, 0)
	if err != nil {
		t.Fatalf("Orders(draft): %v", err)
	}
	for _, o := range drafts {
		if o.Status != procurement.StatusDraft {
			t.Fatalf("the draft filter returned status %q", o.Status)
		}
	}
	one, err := procSvc.Orders(ctx, "", 1, 0)
	if err != nil {
		t.Fatalf("Orders(all, limit 1): %v", err)
	}
	if len(one) != 1 {
		t.Fatalf("a limit of 1 returned %d rows", len(one))
	}
}

// marker turns a loop counter into a letter, so the three orders differ in their numbers.
func marker(i int) string { return string(rune('A' + i)) }
