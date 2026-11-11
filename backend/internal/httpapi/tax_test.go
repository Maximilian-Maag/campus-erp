package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func uniq(t *testing.T, prefix string) string { return prefix + uniqueSuffix(t) }

func TestHTTPVATTypeFlow(t *testing.T) {
	srv, pool := newTestServer(t)
	code := uniq(t, "T")

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+code+`","description":"Regelsteuersatz","rate_basis_points":1900,"kind":"output"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create VAT type = %d", resp.StatusCode)
	}
	var created struct {
		ID              int64 `json:"ID"`
		Code            string
		RateBasisPoints int64
		Kind            string
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.RateBasisPoints != 1900 || created.Kind != "output" {
		t.Fatalf("created = %+v", created)
	}

	resp = doJSON(t, http.MethodGet, srv.URL+"/api/vat-types/"+code, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get VAT type = %d", resp.StatusCode)
	}

	if got := countSubject(t, pool, "vat-type/"+code); got != 1 {
		t.Fatalf("audit events = %d, want 1", got)
	}

	// duplicate -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+code+`","rate_basis_points":1900,"kind":"output"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate VAT type = %d, want 409", resp.StatusCode)
	}
}

func TestHTTPVATTypeErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// unknown kind -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+uniq(t, "T")+`","rate_basis_points":1900,"kind":"sideways"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown kind = %d, want 422", resp.StatusCode)
	}
	// rate out of range
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"`+uniq(t, "T")+`","rate_basis_points":10001,"kind":"output"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rate out of range = %d, want 422", resp.StatusCode)
	}
	// code too long
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice",
		`{"code":"123456789","rate_basis_points":0,"kind":"output"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("code too long = %d, want 422", resp.StatusCode)
	}
	// unknown code -> 404
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/vat-types/nope", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown VAT code = %d, want 404", resp.StatusCode)
	}
}

func TestHTTPTaxOfficeAndFoundation(t *testing.T) {
	srv, _ := newTestServer(t)
	num := uniq(t, "9")

	// tax office with a valid IBAN
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+num+`","name":"Finanzamt Musterstadt","street":"Steuerweg 1",`+
			`"postal_code":"12345","city":"Musterstadt","account_holder":"Land Muster",`+
			`"iban":"DE02120300000000202051","bic":"byladem1001"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tax office = %d", resp.StatusCode)
	}
	var office struct {
		BIC  string
		IBAN string
	}
	if err := json.NewDecoder(resp.Body).Decode(&office); err != nil {
		t.Fatalf("decode office: %v", err)
	}
	if office.BIC != "BYLADEM1001" {
		t.Fatalf("BIC not upper-cased: %q", office.BIC)
	}

	// invalid IBAN -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+uniq(t, "8")+`","name":"Amt","iban":"DE02120300000000202052"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid IBAN = %d, want 422", resp.StatusCode)
	}
	// duplicate -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":2026,"number":"`+num+`","name":"Amt"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate tax office = %d, want 409", resp.StatusCode)
	}
	// malformed body
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad office body = %d, want 400", resp.StatusCode)
	}
	// bad year -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/tax-offices", "alice",
		`{"year":1800,"number":"1","name":"Amt"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad year = %d, want 422", resp.StatusCode)
	}

	// foundation
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+uniq(t, "7")+`","description":"Deutschlandstipendium","carry_over":true}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation = %d", resp.StatusCode)
	}
	fnum := uniq(t, "6")
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+fnum+`","description":"Stiftung"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+fnum+`","description":"Stiftung"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate foundation = %d, want 409", resp.StatusCode)
	}
	// blank description -> validation error
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice",
		`{"year":2026,"number":"`+uniq(t, "5")+`","description":"   "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank description = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad foundation body = %d, want 400", resp.StatusCode)
	}
}

// TestHTTPFoundationsListNarrowsByYear is the read the Stiftungen screen opens with: every year,
// one year, a year nothing carries, and a year that is not a number.
func TestHTTPFoundationsListNarrowsByYear(t *testing.T) {
	srv, _ := newTestServer(t)
	current := foundationInYear(t, srv.URL, 2026)
	previous := foundationInYear(t, srv.URL, 2025)

	all := listFoundations(t, srv.URL+"/api/foundations")
	if !holdsFoundation(all, current.ID) || !holdsFoundation(all, previous.ID) {
		t.Fatalf("every year must hold %d and %d", current.ID, previous.ID)
	}

	only2025 := listFoundations(t, srv.URL+"/api/foundations?year=2025")
	if !holdsFoundation(only2025, previous.ID) {
		t.Fatalf("2025 must hold %d", previous.ID)
	}
	if holdsFoundation(only2025, current.ID) {
		t.Fatalf("2025 must not hold %d, which is a 2026 foundation", current.ID)
	}
	for _, f := range only2025 {
		if f.Year != 2025 {
			t.Fatalf("listing 2025 returned %d/%s", f.Year, f.Number)
		}
	}

	if got := listFoundations(t, srv.URL+"/api/foundations?year=2099"); len(got) != 0 {
		t.Fatalf("2099 = %+v, want none", got)
	}

	// a year that is not a number is refused with a code, not read as every year
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/foundations?year=heuer", "", "")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad year = %d, want 422", resp.StatusCode)
	}
	var problem struct{ Code string }
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "invalid_year" {
		t.Fatalf("code = %q, want invalid_year", problem.Code)
	}
}

// The two tax reads: the VAT-type catalogue by code, and the tax offices narrowed by year. Both
// were write-only routes before — a screen had nothing to read.
func TestHTTPVATTypesAndTaxOfficesList(t *testing.T) {
	srv, _ := newTestServer(t)

	// Two VAT types of this test's own, created through the API.
	codeA := uniq(t, "V7")
	codeB := uniq(t, "V6")
	for _, body := range []string{
		`{"code":"` + codeB + `","description":"ermaessigt","rate_basis_points":700,"kind":"output"}`,
		`{"code":"` + codeA + `","description":"regel","rate_basis_points":1900,"kind":"output"}`,
	} {
		if resp := doJSON(t, http.MethodPost, srv.URL+"/api/vat-types", "alice", body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("create VAT type = %d", resp.StatusCode)
		}
	}
	types := listVATTypes(t, srv.URL)
	if !holdsVATCode(types, codeA) || !holdsVATCode(types, codeB) {
		t.Fatalf("the catalogue must hold %s and %s", codeA, codeB)
	}
	// The catalogue is ordered by code, so a screen shows a stable list.
	for i := 1; i < len(types); i++ {
		if types[i-1].Code > types[i].Code {
			t.Fatalf("VAT types are not in code order: %q before %q", types[i-1].Code, types[i].Code)
		}
	}

	// Two tax offices in two years.
	current := taxOfficeInYear(t, srv.URL, 2026)
	previous := taxOfficeInYear(t, srv.URL, 2025)
	all := listTaxOffices(t, srv.URL+"/api/tax-offices")
	if !holdsTaxOffice(all, current.ID) || !holdsTaxOffice(all, previous.ID) {
		t.Fatalf("every year must hold %d and %d", current.ID, previous.ID)
	}
	only2025 := listTaxOffices(t, srv.URL+"/api/tax-offices?year=2025")
	if !holdsTaxOffice(only2025, previous.ID) || holdsTaxOffice(only2025, current.ID) {
		t.Fatalf("2025 must hold %d and not %d", previous.ID, current.ID)
	}
	for _, o := range only2025 {
		if o.Year != 2025 {
			t.Fatalf("listing 2025 returned %d/%s", o.Year, o.Number)
		}
	}
	// A year that is not a number is refused with a code, not read as every year.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/tax-offices?year=heuer", "", ""); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad year = %d, want 422", resp.StatusCode)
	}
}

type namedID struct {
	ID     int64
	Year   int
	Code   string
	Number string
}

func listVATTypes(t *testing.T, base string) []namedID {
	t.Helper()
	resp := doJSON(t, http.MethodGet, base+"/api/vat-types", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list VAT types = %d", resp.StatusCode)
	}
	var types []namedID
	if err := json.NewDecoder(resp.Body).Decode(&types); err != nil {
		t.Fatalf("decode VAT types: %v", err)
	}
	return types
}

func holdsVATCode(list []namedID, code string) bool {
	for _, v := range list {
		if v.Code == code {
			return true
		}
	}
	return false
}

func listTaxOffices(t *testing.T, url string) []namedID {
	t.Helper()
	resp := doJSON(t, http.MethodGet, url, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list tax offices %s = %d (%s)", url, resp.StatusCode, readBody(t, resp))
	}
	var offices []namedID
	if err := json.NewDecoder(resp.Body).Decode(&offices); err != nil {
		t.Fatalf("decode tax offices: %v", err)
	}
	return offices
}

func holdsTaxOffice(list []namedID, id int64) bool {
	for _, o := range list {
		if o.ID == id {
			return true
		}
	}
	return false
}

// taxOfficeInYear creates a tax office in one year and returns it, so a list test has rows of its
// own in a shared schema.
func taxOfficeInYear(t *testing.T, srv string, year int) namedID {
	t.Helper()
	body := `{"year":` + itoa(int64(year)) + `,"number":"` + uniq(t, "F") + `","name":"Finanzamt Musterstadt"}`
	resp := doJSON(t, http.MethodPost, srv+"/api/tax-offices", "alice", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tax office = %d", resp.StatusCode)
	}
	var office namedID
	if err := json.NewDecoder(resp.Body).Decode(&office); err != nil {
		t.Fatalf("decode tax office: %v", err)
	}
	return office
}

// foundationInYear creates a foundation in the given year and returns what the API wrote.
func foundationInYear(t *testing.T, srv string, year int) foundationJSON {
	t.Helper()
	body := `{"year":` + itoa(int64(year)) + `,"number":"STIFT-` + uniqueSuffix(t) +
		`","description":"Stiftung","carry_over":false}`
	resp := doJSON(t, http.MethodPost, srv+"/api/foundations", "alice", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation %d = %d", year, resp.StatusCode)
	}
	var f foundationJSON
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		t.Fatalf("decode foundation: %v", err)
	}
	return f
}

// listFoundations reads the foundations list, failing the test on anything but 200.
func listFoundations(t *testing.T, url string) []foundationJSON {
	t.Helper()
	resp := doJSON(t, http.MethodGet, url, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	var list []foundationJSON
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode foundations: %v", err)
	}
	return list
}

func holdsFoundation(list []foundationJSON, id int64) bool {
	for _, f := range list {
		if f.ID == id {
			return true
		}
	}
	return false
}
