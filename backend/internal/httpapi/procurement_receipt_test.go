package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// approvedPO creates an order of one line (net 10000, 19 %) and approves it, so a delivery can
// be recorded against it. The gross is 11900.
func approvedPO(t *testing.T, srv string, positionID int64) int64 {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv+"/api/purchase-orders", "alice", poBody("PO-"+uniqueSuffix(t), positionID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create order = %d", resp.StatusCode)
	}
	var order struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if resp := doJSON(t, http.MethodPost, srv+"/api/purchase-orders/"+itoa(order.ID)+"/approve", "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d", resp.StatusCode)
	}
	return order.ID
}

func receiptBody(reference, receivedOn string, lines ...[2]int64) string {
	body := `{"reference":"` + reference + `","received_on":"` + receivedOn + `","lines":[`
	for i, l := range lines {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"line_no":%d,"quantity_milli":%d}`, l[0], l[1])
	}
	return body + `]}`
}

type receiptResultJSON struct {
	Receipt struct {
		ID            int64
		Reference     string
		ReceivedCents int64
		Lines         []struct {
			LineNo        int
			QuantityMilli int64
		}
	}
	State receiptStateJSON
}

type receiptStateJSON struct {
	OrderID  int64
	Complete bool
	Progress []struct {
		LineNo        int
		OrderedMilli  int64
		ReceivedMilli int64
	}
	Receipts []struct {
		ID            int64
		Reference     string
		ReceivedCents int64
	}
}

// TestHTTPGoodsReceiptFlow drives the whole path through the real service and database: an
// approved order, a partial delivery that converts part of its commitment, the delivery that
// completes it, and the order's arrival state as the screen reads it.
func TestHTTPGoodsReceiptFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	posID := newBudgetPosition(t, srv.URL)
	id := approvedPO(t, srv.URL, posID)
	base := srv.URL + "/api/purchase-orders/" + itoa(id)

	// Before any delivery: one line, nothing received, not complete.
	resp := doJSON(t, http.MethodGet, base+"/receipts", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state = %d", resp.StatusCode)
	}
	var state receiptStateJSON
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.OrderID != id || state.Complete || len(state.Receipts) != 0 {
		t.Fatalf("state before any delivery = %+v", state)
	}
	if len(state.Progress) != 1 || state.Progress[0].OrderedMilli != 1000 || state.Progress[0].ReceivedMilli != 0 {
		t.Fatalf("progress before any delivery = %+v", state.Progress)
	}

	// Half of the line arrives: net 5000 plus 19 % = 5950.
	first := "LS-1-" + uniqueSuffix(t)
	resp = doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody(first, "2026-11-20", [2]int64{1, 500}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first receipt = %d", resp.StatusCode)
	}
	var result receiptResultJSON
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Receipt.Reference != first || result.Receipt.ReceivedCents != 5950 || result.Receipt.ID == 0 {
		t.Fatalf("receipt = %+v", result.Receipt)
	}
	if len(result.Receipt.Lines) != 1 || result.Receipt.Lines[0].QuantityMilli != 500 {
		t.Fatalf("receipt lines = %+v", result.Receipt.Lines)
	}
	if result.State.Complete || result.State.Progress[0].ReceivedMilli != 500 {
		t.Fatalf("state after half a delivery = %+v", result.State)
	}
	if len(result.State.Receipts) != 1 || result.State.Receipts[0].Reference != first ||
		result.State.Receipts[0].ReceivedCents != 5950 {
		t.Fatalf("receipts after the first delivery = %+v", result.State.Receipts)
	}

	// The rest arrives: the order is complete, and this receipt converts the remainder, so the
	// two together are the order's gross exactly.
	resp = doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-2-"+uniqueSuffix(t), "2026-11-25", [2]int64{1, 500}))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second receipt = %d", resp.StatusCode)
	}
	var final receiptResultJSON
	if err := json.NewDecoder(resp.Body).Decode(&final); err != nil {
		t.Fatalf("decode final: %v", err)
	}
	if !final.State.Complete || len(final.State.Receipts) != 2 {
		t.Fatalf("final state = %+v", final.State)
	}
	if got := final.Receipt.ReceivedCents + result.Receipt.ReceivedCents; got != 11900 {
		t.Fatalf("the receipts convert %d, want the order's gross 11900", got)
	}

	// The order now reads as received, which is what the list shows.
	resp = doJSON(t, http.MethodGet, base, "alice", "")
	var order struct{ Status string }
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order.Status != "received" {
		t.Fatalf("order status = %s, want received", order.Status)
	}
}

// TestHTTPGoodsReceiptRefusals covers the refusals at the edge: what the transport refuses before
// the domain ever sees it, and what the domain refuses, each with its own status.
func TestHTTPGoodsReceiptRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	posID := newBudgetPosition(t, srv.URL)
	id := approvedPO(t, srv.URL, posID)
	base := srv.URL + "/api/purchase-orders/" + itoa(id)

	// A body that is not JSON, and a date that is not a day.
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-X", "20.11.2026", [2]int64{1, 100})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad date = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-X", "", [2]int64{1, 100})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing date = %d, want 422", resp.StatusCode)
	}
	// A line the order does not carry, no lines at all, and a quantity of nothing.
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-X", "2026-11-20", [2]int64{9, 100})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown line = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", `{"reference":"LS-X","received_on":"2026-11-20","lines":[]}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no lines = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-X", "2026-11-20", [2]int64{1, 0})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("zero quantity = %d, want 422", resp.StatusCode)
	}
	// Over-receipt: the whole line plus one milli-unit.
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-X", "2026-11-20", [2]int64{1, 1001})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("over-receipt = %d, want 422", resp.StatusCode)
	}

	// An accepted delivery, then the same delivery note again.
	reference := "LS-DUP-" + uniqueSuffix(t)
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody(reference, "2026-11-20", [2]int64{1, 400})); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first delivery = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody(reference, "2026-11-21", [2]int64{1, 100})); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate reference = %d, want 409", resp.StatusCode)
	}
	// More than is still open on the line: 400 arrived of 1000, so 601 does not fit.
	if resp := doJSON(t, http.MethodPost, base+"/receipts", "carol", receiptBody("LS-OVER", "2026-11-21", [2]int64{1, 601})); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("over the earlier delivery = %d, want 422", resp.StatusCode)
	}

	// An order that is still a draft committed nothing, so it cannot receive goods.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders", "alice", poBody("PO-D-"+uniqueSuffix(t), posID))
	var draft struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/purchase-orders/"+itoa(draft.ID)+"/receipts", "carol",
		receiptBody("LS-D", "2026-11-20", [2]int64{1, 100})); resp.StatusCode != http.StatusConflict {
		t.Fatalf("delivery on a draft = %d, want 409", resp.StatusCode)
	}

	// An unknown order, and a path that is not an id.
	unknown := srv.URL + "/api/purchase-orders/9223372036854775807/receipts"
	if resp := doJSON(t, http.MethodPost, unknown, "carol", receiptBody("LS-U", "2026-11-20", [2]int64{1, 100})); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order (post) = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, unknown, "carol", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown order (get) = %d, want 404", resp.StatusCode)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if resp := doJSON(t, method, srv.URL+"/api/purchase-orders/abc/receipts", "carol", receiptBody("LS-U", "2026-11-20", [2]int64{1, 100})); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s with a non-numeric id = %d, want 400", method, resp.StatusCode)
		}
	}

	// The refusals left the order as it was: 400 of 1000 received, not complete, one receipt.
	resp = doJSON(t, http.MethodGet, base+"/receipts", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state = %d", resp.StatusCode)
	}
	var state receiptStateJSON
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Complete || len(state.Receipts) != 1 || state.Receipts[0].Reference != reference {
		t.Fatalf("state after the refusals = %+v", state)
	}
	if state.Progress[0].ReceivedMilli != 400 {
		t.Fatalf("received after the refusals = %+v", state.Progress)
	}
}
