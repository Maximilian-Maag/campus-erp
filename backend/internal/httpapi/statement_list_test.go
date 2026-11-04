package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// createAccount stores a cash account and returns it, so the list tests have one to name.
func createAccount(t *testing.T, srvURL, name string) (int64, accountJSON) {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srvURL+"/api/cash-accounts", "carol",
		`{"name":"`+name+`","iban":"DE02120300000000202051","bic":"BYLADEM1001","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create account = %d", resp.StatusCode)
	}
	var account accountJSON
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	return account.ID, account
}

// TestHTTPCashAccountList: the accounts a screen offers before it can ask about one.
func TestHTTPCashAccountList(t *testing.T) {
	srv, _ := newTestServer(t)
	id, _ := createAccount(t, srv.URL, "Universität")

	resp := doJSON(t, http.MethodGet, srv.URL+"/api/cash-accounts", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list accounts = %d", resp.StatusCode)
	}
	var accounts []struct {
		ID       int64
		Name     string
		IBAN     string
		Currency string
	}
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	if len(accounts) == 0 {
		t.Fatal("no accounts listed")
	}
	var found bool
	for _, a := range accounts {
		if a.ID != id {
			continue
		}
		found = true
		if a.Name != "Universität" || a.IBAN == "" || a.Currency != "EUR" {
			t.Fatalf("listed account = %+v", a)
		}
	}
	if !found {
		t.Fatalf("the account just created is not among %d", len(accounts))
	}
}

// mt940 returns a minimal statement with the given reference, which is what makes a second import
// a second statement rather than the same file seen twice.
func mt940(reference string) string {
	return ":20:" + reference + "\n" +
		":25:DE02120300000000202051\n:28C:00001/001\n:60F:C261001EUR10000,00\n" +
		":62F:C261031EUR10000,00\n-\n"
}

type statementSummaryJSON struct {
	ID                  int64
	AccountID           int64
	Reference           string
	Number              int
	LineCount           int
	OpeningBalanceCents int64
	ClosingDate         string
	ImportedBy          string
}

func listStatements(t *testing.T, srvURL, query string) []statementSummaryJSON {
	t.Helper()
	resp := doJSON(t, http.MethodGet, srvURL+"/api/bank-statements"+query, "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list statements%s = %d", query, resp.StatusCode)
	}
	var out []statementSummaryJSON
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode statements: %v", err)
	}
	return out
}

// TestHTTPBankStatementList: the imported statements a screen lists, and the account filter that
// belongs to the server, so the screen shows one account's without holding every statement.
func TestHTTPBankStatementList(t *testing.T) {
	srv, _ := newTestServer(t)
	account, _ := createAccount(t, srv.URL, "Universität")

	first := importStatement(t, srv.URL, account, mt940("STMT-A-"+uniqueSuffix(t)))
	second := importStatement(t, srv.URL, account, mt940("STMT-B-"+uniqueSuffix(t)))

	all := listStatements(t, srv.URL, "")
	if len(all) < 2 {
		t.Fatalf("list = %d statements, want at least the two just imported", len(all))
	}
	for _, want := range []int64{first.ID, second.ID} {
		var found bool
		for _, s := range all {
			if s.ID != want {
				continue
			}
			found = true
			if s.AccountID != account || s.Reference == "" {
				t.Fatalf("listed statement = %+v", s)
			}
			if s.Number != 1 || s.LineCount != 0 || s.OpeningBalanceCents != 1000000 {
				t.Fatalf("listed statement = %+v", s)
			}
			if s.ImportedBy != "carol" || s.ClosingDate == "" {
				t.Fatalf("listed statement = %+v", s)
			}
		}
		if !found {
			t.Fatalf("statement %d is not among %d", want, len(all))
		}
	}

	// The filter is the server's: only that account's statements come back.
	filtered := listStatements(t, srv.URL, "?account_id="+itoa(account))
	if len(filtered) < 2 {
		t.Fatalf("filtered list = %d, want the account's two statements", len(filtered))
	}
	for _, s := range filtered {
		if s.AccountID != account {
			t.Fatalf("filtered list contains a statement of account %d", s.AccountID)
		}
	}

	// An account with no statements is an empty answer, not an error.
	if none := listStatements(t, srv.URL, "?account_id=9223372036854775807"); len(none) != 0 {
		t.Fatalf("an account with no statements returned %d", len(none))
	}
}

// TestHTTPBankStatementListBadFilter: an account that is not a number is refused rather than
// silently read as "all accounts", which would look like a working filter and not be one.
func TestHTTPBankStatementListBadFilter(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, bad := range []string{"abc", "-1"} {
		resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements?account_id="+bad, "carol", "")
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("account_id=%q = %d, want 422", bad, resp.StatusCode)
		}
	}
	// An empty filter parameter means "all", which is not the same as a malformed one.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements?account_id=", "carol", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("empty account_id = %d, want 200", resp.StatusCode)
	}
	// So does an explicit zero: it is the filter's "all accounts", not a refusal.
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements?account_id=0", "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("account_id=0 = %d, want 200", resp.StatusCode)
	}
	var all []struct {
		ID int64
	}
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// It is the same answer as leaving the filter out, which the test above already listed from.
	unfiltered := listStatements(t, srv.URL, "")
	if len(all) != len(unfiltered) {
		t.Fatalf("account_id=0 returned %d statements, the unfiltered list %d", len(all), len(unfiltered))
	}
}
