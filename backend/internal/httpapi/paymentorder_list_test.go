package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestHTTPPaymentOrderList: the list a screen reads, and the status filter that belongs to the
// server so the screen never has to hold every order to work on part of them.
func TestHTTPPaymentOrderList(t *testing.T) {
	srv, _ := newTestServer(t)

	// One order left in draft, one submitted.
	draft := createOrder(t, srv.URL, "alice")
	submitted := createOrder(t, srv.URL, "alice")
	if resp := doJSON(t, http.MethodPost,
		fmt.Sprintf("%s/api/payment-orders/%d/submit", srv.URL, submitted.ID), "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("submit = %d", resp.StatusCode)
	}

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-orders", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list = %d", resp.StatusCode)
	}
	var all []struct {
		ID          int64
		Reference   string
		AmountCents int64
		Currency    string
		Purpose     string
		DueDate     string
		Status      string
	}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("list = %d orders, want at least the two just created", len(all))
	}
	// The screen shows what an order is for, how much and when it is due.
	var found bool
	for _, o := range all {
		if o.ID != draft.ID {
			continue
		}
		found = true
		if o.AmountCents != 125000 || o.Currency != "EUR" || o.Purpose != "Stromabschlag" {
			t.Fatalf("listed order = %+v", o)
		}
		if o.Status != "draft" || o.Reference == "" {
			t.Fatalf("listed order = %+v", o)
		}
		if _, err := time.Parse(time.RFC3339, o.DueDate); err != nil {
			t.Fatalf("due date %q is not a timestamp: %v", o.DueDate, err)
		}
	}
	if !found {
		t.Fatalf("the draft order is not in the list of %d", len(all))
	}

	// The filter is the server's: only submitted orders come back.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/payment-orders?status=submitted", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("filtered list = %d", resp.StatusCode)
	}
	var submittedOnly []struct {
		ID     int64
		Status string
	}
	if err := json.NewDecoder(resp.Body).Decode(&submittedOnly); err != nil {
		t.Fatalf("decode filtered list: %v", err)
	}
	if len(submittedOnly) == 0 {
		t.Fatal("the submitted order is not in the filtered list")
	}
	for _, o := range submittedOnly {
		if o.Status != "submitted" {
			t.Fatalf("filtered list contains %+v", o)
		}
		if o.ID == draft.ID {
			t.Fatal("the draft order came back through the submitted filter")
		}
	}
}

// TestHTTPPaymentOrderListUnknownStatus: an unknown status is an empty answer rather than a
// failure — nothing matches it, and "no orders carry that state" is the truth.
//
// The route is mounted with authorizeRead(..., auth.RoleKasse) like the rest of the kasse, and
// the test server runs without a verifier, so the role gate itself is exercised in authz_test.go
// rather than here.
func TestHTTPPaymentOrderListUnknownStatus(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/payment-orders?status=nonsense", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unknown status = %d, want an empty 200", resp.StatusCode)
	}
	var orders []struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("unknown status returned %d orders", len(orders))
	}
}
