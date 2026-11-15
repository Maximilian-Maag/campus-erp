package klr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// TestCostTypeBridge: the cost-type catalogue and the bridge from a ledger account code to it.
func TestCostTypeBridge(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	personal, err := svc.PostCostType(ctx, "4711-"+mark, "Personalkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}
	sach, err := svc.PostCostType(ctx, "5000-"+mark, "Sachkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}
	if personal.ID == sach.ID || personal.Code == sach.Code {
		t.Fatalf("two cost types share an identity: %+v / %+v", personal, sach)
	}

	// The code is the identity of a cost type.
	if _, err := svc.PostCostType(ctx, "4711-"+mark, "Doppelt", "alice"); !errors.Is(err, klr.ErrCostTypeExists) {
		t.Fatalf("duplicate code = %v, want ErrCostTypeExists", err)
	}

	types, err := svc.CostTypes(ctx, 200, 0)
	if err != nil {
		t.Fatalf("CostTypes: %v", err)
	}
	var havePersonal, haveSach bool
	for _, ct := range types {
		if ct.ID == personal.ID {
			havePersonal = true
		}
		if ct.ID == sach.ID {
			haveSach = true
		}
	}
	if !havePersonal || !haveSach {
		t.Fatalf("the new cost types are missing from the list of %d", len(types))
	}

	// The bridge: an account belongs to exactly one cost type.
	account := "6810-" + mark
	link, err := svc.MapAccount(ctx, personal.ID, account, "alice")
	if err != nil {
		t.Fatalf("MapAccount: %v", err)
	}
	if link.CostTypeID != personal.ID || link.AccountCode != account {
		t.Fatalf("link = %+v", link)
	}

	// Mapping the same account to the same cost type again is idempotent.
	again, err := svc.MapAccount(ctx, personal.ID, account, "alice")
	if err != nil {
		t.Fatalf("re-map: %v", err)
	}
	if again.ID != link.ID {
		t.Fatalf("re-map made a second link: %+v vs %+v", again, link)
	}

	// Mapping it to another cost type is refused: the bridge is a function, not a bag of links.
	if _, err := svc.MapAccount(ctx, sach.ID, account, "alice"); !errors.Is(err, klr.ErrAccountAlreadyMapped) {
		t.Fatalf("second mapping = %v, want ErrAccountAlreadyMapped", err)
	}

	// An unknown cost type is not a place to map an account.
	if _, err := svc.MapAccount(ctx, 9223372036854775807, "9999-"+mark, "alice"); !errors.Is(err, klr.ErrCostTypeNotFound) {
		t.Fatalf("unknown cost type = %v, want ErrCostTypeNotFound", err)
	}

	// The bridge reads both ways.
	accounts, err := svc.AccountsOf(ctx, personal.ID)
	if err != nil {
		t.Fatalf("AccountsOf: %v", err)
	}
	found := false
	for _, a := range accounts {
		if a.AccountCode == account {
			found = true
		}
	}
	if !found {
		t.Fatalf("the account is not listed under the cost type (%d links)", len(accounts))
	}

	byAccount, err := svc.CostTypeByAccount(ctx, account)
	if err != nil {
		t.Fatalf("CostTypeByAccount: %v", err)
	}
	if byAccount.ID != personal.ID {
		t.Fatalf("by-account = %+v, want %d", byAccount, personal.ID)
	}
	if _, err := svc.CostTypeByAccount(ctx, "unmapped-"+mark); !errors.Is(err, klr.ErrAccountNotMapped) {
		t.Fatalf("unmapped account = %v, want ErrAccountNotMapped", err)
	}
	// A cost type that does not exist has no accounts, rather than an empty list that reads as
	// "it owns nothing".
	if _, err := svc.AccountsOf(ctx, 9223372036854775807); !errors.Is(err, klr.ErrCostTypeNotFound) {
		t.Fatalf("unknown cost type accounts = %v, want ErrCostTypeNotFound", err)
	}

	if err := audit.NewStore(testPool(t)).Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

// TestRecordCarriesItsCostType: a record may name the cost type it belongs to, and the store
// refuses one that does not exist.
func TestRecordCarriesItsCostType(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	center, err := svc.CreateCostCenter(ctx, "K-"+mark, "Verwaltung", "alice")
	if err != nil {
		t.Fatalf("CreateCostCenter: %v", err)
	}
	ct, err := svc.PostCostType(ctx, "4711-"+mark, "Personalkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}

	rec, err := svc.Post(ctx, center.ID, nil, &ct.ID, klr.Cost, 1250, klrDay(2026, 4, 30), "RE-"+mark, "alice")
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if rec.CostTypeID == nil || *rec.CostTypeID != ct.ID {
		t.Fatalf("record cost type = %v, want %d", rec.CostTypeID, ct.ID)
	}

	// A record may carry no cost type (nobody has classified it yet).
	if _, err := svc.Post(ctx, center.ID, nil, nil, klr.Cost, 100, klrDay(2026, 4, 30), "RE2-"+mark, "alice"); err != nil {
		t.Fatalf("Post without a cost type: %v", err)
	}

	// A cost type that does not exist is refused by the store.
	bogus := int64(9223372036854775807)
	if _, err := svc.Post(ctx, center.ID, nil, &bogus, klr.Cost, 100, klrDay(2026, 4, 30), "RE3-"+mark, "alice"); !errors.Is(err, klr.ErrCostTypeNotFound) {
		t.Fatalf("unknown cost type = %v, want ErrCostTypeNotFound", err)
	}
}

// TestCostTypesPage: the catalogue answers the page it was asked for, and walking the pages
// reproduces a window of it in order, with nothing repeated and nothing skipped. The order is by
// code, which is UNIQUE, so a page boundary is safe.
func TestCostTypesPage(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	mark := suffix(t)

	// Four types of this test's own, whose codes share a prefix so they sort together.
	for _, letter := range []string{"A", "B", "C", "D"} {
		if _, err := svc.PostCostType(ctx, "PG-"+mark+"-"+letter, "Paging "+letter, "alice"); err != nil {
			t.Fatalf("PostCostType(%s): %v", letter, err)
		}
	}

	const window = 4
	head, err := svc.CostTypes(ctx, window, 0)
	if err != nil {
		t.Fatalf("CostTypes: %v", err)
	}
	if len(head) < 2 {
		t.Fatalf("the catalogue holds %d types, so nothing about paging is being tested", len(head))
	}

	// The same window as two pages: a list that ignored the offset would repeat the first page, one
	// that ignored the limit would answer with more than was asked for.
	const pageSize = 2
	first, err := svc.CostTypes(ctx, pageSize, 0)
	if err != nil {
		t.Fatalf("CostTypes(offset=0): %v", err)
	}
	second, err := svc.CostTypes(ctx, pageSize, pageSize)
	if err != nil {
		t.Fatalf("CostTypes(offset=%d): %v", pageSize, err)
	}
	if len(first) > pageSize || len(second) > pageSize {
		t.Fatalf("a page of %d holds %d and %d rows", pageSize, len(first), len(second))
	}
	paged := append(append([]klr.CostType{}, first...), second...)
	if len(paged) != len(head) {
		t.Fatalf("two pages of %d yielded %d rows, want the %d of one window", pageSize, len(paged), len(head))
	}
	for i := range head {
		if paged[i].Code != head[i].Code {
			t.Fatalf("the pages diverge from the window at %d: got %q, want %q", i, paged[i].Code, head[i].Code)
		}
	}

	// A page past the end is empty rather than an error.
	past, err := svc.CostTypes(ctx, 5, 100000)
	if err != nil {
		t.Fatalf("CostTypes(past the end): %v", err)
	}
	if len(past) != 0 {
		t.Fatalf("a page past the end holds %d rows", len(past))
	}
}
