package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type statementJSON struct {
	ID        int64
	LineCount int
	Lines     []struct {
		LineNo int
		Status string
	}
}

// importStatement posts an MT940 file and returns the first statement.
func importStatement(t *testing.T, srv string, accountID int64, content string) statementJSON {
	t.Helper()
	body, err := json.Marshal(map[string]any{"account_id": accountID, "content": content})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := doJSON(t, http.MethodPost, srv+"/api/bank-statements", "carol", string(body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import statement = %d", resp.StatusCode)
	}
	var stmts []statementJSON
	if err := json.NewDecoder(resp.Body).Decode(&stmts); err != nil {
		t.Fatalf("decode statements: %v", err)
	}
	if len(stmts) == 0 {
		t.Fatal("no statement returned")
	}
	return stmts[0]
}

func TestHTTPStatementImportFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	// A debtor account.
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/bank-accounts", "carol",
		`{"name":"Universität","iban":"DE02120300000000202051","bic":"BYLADEM1001","currency":"EUR"}`)
	var account accountJSON
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	// A paid order (create -> submit -> approve -> run).
	order := createApprovedOrder(t, srv.URL, 125000)
	runBody := `{"reference":"RUN-` + uniqueSuffix(t) + `","account_id":` + itoa(account.ID) +
		`,"execution_date":"2026-10-28","order_ids":[` + itoa(order.ID) + `]}`
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/payment-runs", "carol", runBody); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create run = %d", resp.StatusCode)
	}

	content := ":20:STMT-" + uniqueSuffix(t) + "\n" +
		":25:DE02120300000000202051\n:28C:00001/001\n:60F:C261001EUR10000,00\n" +
		":61:2610011001D1250,00N051" + order.Reference + "//BANKREF\n" +
		":86:Zahlung\n" +
		":62F:C261031EUR10000,00\n-\n"

	st := importStatement(t, srv.URL, account.ID, content)
	if st.LineCount != 1 {
		t.Fatalf("line count = %d, want 1", st.LineCount)
	}
	if len(st.Lines) != 1 || st.Lines[0].Status != "matched" {
		t.Fatalf("lines = %+v, want one matched line", st.Lines)
	}

	// Fetch it back.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements/"+itoa(st.ID), "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get statement = %d", resp.StatusCode)
	}
	var loaded statementJSON
	if err := json.NewDecoder(resp.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode loaded: %v", err)
	}
	if loaded.ID != st.ID || len(loaded.Lines) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}

	// Idempotent re-import.
	again := importStatement(t, srv.URL, account.ID, content)
	if again.ID != st.ID {
		t.Fatalf("re-import id = %d, want %d", again.ID, st.ID)
	}
}

func TestHTTPStatementErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)

	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/bank-statements", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/bank-statements", "a",
		`{"account_id":1,"content":"   "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty content = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/bank-statements", "a",
		`{"account_id":0,"content":":20:A\n:60F:C260101EUR0,00\n:62F:C260101EUR0,00\n"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("no account = %d, want 422", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements/9223372036854775807", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown statement = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/bank-statements/abc", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	// A malformed MT940 file -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/bank-statements", "a",
		`{"account_id":1,"content":":20:A\n:61:broken\n"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid file = %d, want 422", resp.StatusCode)
	}
}
