package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
)

// TestMatchEInvoiceRoute is the match over the API: an approved purchase order, the supplier's
// invoice imported through the route, and the match converting its commitment into an actual.
func TestMatchEInvoiceRoute(t *testing.T) {
	srv, pool := newTestServer(t)
	ctx := context.Background()
	store := audit.NewStore(pool)
	budgetSvc := budget.NewPositionService(budget.NewPositionRepo(pool), store)
	procSvc := procurement.NewOrderService(procurement.NewOrderRepo(pool), budgetSvc, store)

	pos, err := budgetSvc.CreatePosition(ctx, 2026, "0710", "6810-"+uniqueSuffix(t), "Beschaffung", 1000000, "alice")
	if err != nil {
		t.Fatalf("CreatePosition: %v", err)
	}
	line := procurement.Line{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000,
		UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}
	order, err := procSvc.Create(ctx, "PO-"+uniqueSuffix(t), "Lieferant GmbH", pos.ID, "EUR", []procurement.Line{line}, "alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := procSvc.Approve(ctx, order.ID, "dave"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// The supplier's invoice arrives through the API, naming the order as its buyer reference.
	inv := core.Invoice{
		Number: "RE-" + uniqueSuffix(t), IssueDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		TypeCode: "380", Currency: "EUR", BuyerReference: order.Number,
		Seller: core.Party{Name: "Lieferant GmbH", Street: "Musterweg 1", City: "Musterstadt",
			PostalCode: "12345", Country: "DE", VatID: "DE123456789",
			EndpointID: "rechnung@lieferant.de", EndpointScheme: "EM", Email: "rechnung@lieferant.de"},
		Buyer: core.Party{Name: "Stadt Musterstadt", Street: "Rathausplatz 1", City: "Musterstadt",
			PostalCode: "12345", Country: "DE"},
		PaymentMeansCode: "58", PayeeIBAN: "DE75512108001245126199",
		PaymentTerms: "Zahlbar innerhalb von 30 Tagen ohne Abzug.",
		Lines: []core.Line{{LineNo: 1, Description: "Druckleistung", QuantityMilli: 1000, UnitCode: "C62",
			UnitPriceCents: 30000, NetCents: 30000, VatCategory: "S", VatRateBP: 1900}},
	}
	doc, err := core.GenerateXRechnung(inv)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices/received", "alice", doc)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import status = %d, want 201", resp.StatusCode)
	}
	var received struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&received); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if received.ID == 0 {
		t.Fatal("imported invoice has no id")
	}

	// Match it.
	url := fmt.Sprintf("%s/api/e-invoices/%d/match", srv.URL, received.ID)
	if resp := doJSON(t, http.MethodPost, url, "alice", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("match status = %d, want 200", resp.StatusCode)
	}
	got, err := procSvc.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Status) != "received" {
		t.Fatalf("order status = %s, want received", got.Status)
	}
	bal, err := budgetSvc.Balance(ctx, pos.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.CommittedCents != 0 || bal.ActualCents != 35700 {
		t.Fatalf("balance = committed %d / actual %d, want 0 / 35700", bal.CommittedCents, bal.ActualCents)
	}

	// Matching the same invoice again is a conflict: its order already left the approved state.
	if resp := doJSON(t, http.MethodPost, url, "alice", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second match status = %d, want 409", resp.StatusCode)
	}
	// An invoice that does not exist.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices/9223372036854775807/match", "alice", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown invoice status = %d, want 404", resp.StatusCode)
	}
	// A malformed id.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/e-invoices/nope/match", "alice", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", resp.StatusCode)
	}
}
