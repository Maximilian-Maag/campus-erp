package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestHTTPInternalPricing drives internal pricing over HTTP: a rate is defined for a cost type and
// a service is charged out at it.
func TestHTTPInternalPricing(t *testing.T) {
	srv, _ := newTestServer(t)
	mark := uniqueSuffix(t)

	provider := createCenter(t, srv.URL, "alice", "P")
	receiver := createCenter(t, srv.URL, "alice", "R")
	costType := createCostType(t, srv.URL+"/api/klr/cost-types", "4711-"+mark, "Personalkosten")

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/klr/service-rates", "alice",
		`{"cost_type_id":`+itoa(costType)+`,"rate_cents":2500,"valid_from":"2026-01-01"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("define rate = %d, want 201", resp.StatusCode)
	}
	var rate struct {
		ID         int64
		CostTypeID int64
		RateCents  int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&rate); err != nil {
		t.Fatalf("decode rate: %v", err)
	}
	if rate.ID == 0 || rate.CostTypeID != costType || rate.RateCents != 2500 {
		t.Fatalf("rate = %+v", rate)
	}

	// The list a screen reads before it can charge anything.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/klr/service-rates", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list rates = %d", resp.StatusCode)
	}
	var rates []struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		t.Fatalf("decode rates: %v", err)
	}
	found := false
	for _, r := range rates {
		if r.ID == rate.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the new rate is missing from the list of %d", len(rates))
	}

	// Three units at 25,00 is 75,00.
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/klr/service-charges", "alice",
		`{"provider_center_id":`+itoa(provider)+`,"receiver_center_id":`+itoa(receiver)+
			`,"cost_type_id":`+itoa(costType)+`,"quantity":3,"charged_on":"2026-04-30"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("charge = %d, want 201", resp.StatusCode)
	}
	var charge struct {
		ID          int64
		Quantity    int64
		AmountCents int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&charge); err != nil {
		t.Fatalf("decode charge: %v", err)
	}
	if charge.ID == 0 || charge.Quantity != 3 || charge.AmountCents != 7500 {
		t.Fatalf("charge = %+v", charge)
	}
}

// TestHTTPInternalPricingRefusals: the statuses a screen reads for the mistakes it can make.
func TestHTTPInternalPricingRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	mark := uniqueSuffix(t)

	provider := createCenter(t, srv.URL, "alice", "P")
	receiver := createCenter(t, srv.URL, "alice", "R")
	costType := createCostType(t, srv.URL+"/api/klr/cost-types", "5000-"+mark, "Sachkosten")

	ratesURL := srv.URL + "/api/klr/service-rates"
	chargesURL := srv.URL + "/api/klr/service-charges"

	if resp := doJSON(t, http.MethodPost, ratesURL, "alice",
		`{"cost_type_id":9223372036854775807,"rate_cents":2500,"valid_from":"2026-01-01"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown cost type = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, ratesURL, "alice",
		`{"cost_type_id":`+itoa(costType)+`,"rate_cents":0,"valid_from":"2026-01-01"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("zero rate = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, ratesURL, "alice",
		`{"cost_type_id":`+itoa(costType)+`,"rate_cents":2500,"valid_from":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_from = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, ratesURL, "alice",
		`{"cost_type_id":`+itoa(costType)+`,"rate_cents":2500,"valid_from":"2026-01-01","valid_to":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad valid_to = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, ratesURL, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}

	// No rate in force means nothing to charge.
	if resp := doJSON(t, http.MethodPost, chargesURL, "alice",
		`{"provider_center_id":`+itoa(provider)+`,"receiver_center_id":`+itoa(receiver)+
			`,"cost_type_id":`+itoa(costType)+`,"quantity":1,"charged_on":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no rate = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, chargesURL, "alice",
		`{"provider_center_id":`+itoa(provider)+`,"receiver_center_id":`+itoa(receiver)+
			`,"cost_type_id":`+itoa(costType)+`,"quantity":1,"charged_on":"nonsense"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad charged_on = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, chargesURL, "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", resp.StatusCode)
	}

	// With a rate in force, a centre charging itself and a charge for nothing are both refused.
	if resp := doJSON(t, http.MethodPost, ratesURL, "alice",
		`{"cost_type_id":`+itoa(costType)+`,"rate_cents":2500,"valid_from":"2026-01-01"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("define rate = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, chargesURL, "alice",
		`{"provider_center_id":`+itoa(provider)+`,"receiver_center_id":`+itoa(provider)+
			`,"cost_type_id":`+itoa(costType)+`,"quantity":1,"charged_on":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("self charge = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, chargesURL, "alice",
		`{"provider_center_id":`+itoa(provider)+`,"receiver_center_id":`+itoa(receiver)+
			`,"cost_type_id":`+itoa(costType)+`,"quantity":0,"charged_on":"2026-04-30"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no quantity = %d, want 422", resp.StatusCode)
	}
}
