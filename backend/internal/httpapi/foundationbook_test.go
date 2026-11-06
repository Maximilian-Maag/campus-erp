package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type bookJSON struct {
	ID           int64
	FoundationID int64
	Reference    string
}

type foundationJSON struct {
	ID     int64
	Year   int
	Number string
}

type balanceJSON struct {
	BookID       int64
	IncomeCents  int64
	ExpenseCents int64
}

func createFoundation(t *testing.T, srv string) foundationJSON {
	t.Helper()
	body := `{"year":2026,"number":"STIFT-` + uniqueSuffix(t) + `","description":"Stiftung","carry_over":false}`
	resp := doJSON(t, http.MethodPost, srv+"/api/foundations", "alice", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create foundation = %d", resp.StatusCode)
	}
	var f foundationJSON
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		t.Fatalf("decode foundation: %v", err)
	}
	return f
}

func TestHTTPFoundationBookFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	f := createFoundation(t, srv.URL)

	// Create two books for two foundations and verify isolation over HTTP.
	f2 := createFoundation(t, srv.URL)

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "alice",
		`{"foundation_id":`+itoa(f.ID)+`,"reference":"B-`+uniqueSuffix(t)+`","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create book = %d", resp.StatusCode)
	}
	var book bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&book); err != nil {
		t.Fatalf("decode book: %v", err)
	}
	if book.FoundationID != f.ID {
		t.Fatalf("book foundation = %d, want %d", book.FoundationID, f.ID)
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "alice",
		`{"foundation_id":`+itoa(f2.ID)+`,"reference":"B-`+uniqueSuffix(t)+`","currency":"EUR"}`)
	var book2 bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&book2); err != nil {
		t.Fatalf("decode book2: %v", err)
	}

	// Book movements.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/bookings", "alice",
		`{"kind":"income","amount_cents":10000,"purpose":"Spende"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("book income = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/bookings", "alice",
		`{"kind":"expense","amount_cents":3000,"purpose":"Auszahlung"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("book expense = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book2.ID)+"/bookings", "alice",
		`{"kind":"income","amount_cents":50000,"purpose":"Spende"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("book2 income = %d", resp.StatusCode)
	}

	// Bookings scoped to the book.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/bookings", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get bookings = %d", resp.StatusCode)
	}
	var bookings []struct{ BookID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&bookings); err != nil {
		t.Fatalf("decode bookings: %v", err)
	}
	if len(bookings) != 2 {
		t.Fatalf("bookings = %d, want 2", len(bookings))
	}
	for _, b := range bookings {
		if b.BookID != book.ID {
			t.Fatalf("leaked booking of book %d", b.BookID)
		}
	}

	// Balance independent.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/balance", "alice", "")
	var bal balanceJSON
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if bal.IncomeCents != 10000 || bal.ExpenseCents != 3000 {
		t.Fatalf("balance = %+v", bal)
	}
}

func TestHTTPFoundationBookErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	f := createFoundation(t, srv.URL)

	// Malformed body.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	// Unknown foundation -> 404.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "a",
		`{"foundation_id":9223372036854775807,"reference":"x`+uniqueSuffix(t)+`","currency":"EUR"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown foundation = %d, want 404", resp.StatusCode)
	}
	// Bad currency -> 422.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "a",
		`{"foundation_id":`+itoa(f.ID)+`,"reference":"x","currency":"EURO"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad currency = %d, want 422", resp.StatusCode)
	}
	// Duplicate reference -> 409.
	ref := "DUP-" + uniqueSuffix(t)
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "a",
		`{"foundation_id":`+itoa(f.ID)+`,"reference":"`+ref+`","currency":"EUR"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first book = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books", "a",
		`{"foundation_id":`+itoa(f.ID)+`,"reference":"`+ref+`","currency":"EUR"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate book = %d, want 409", resp.StatusCode)
	}
	// Unknown book for bookings -> 404.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/9223372036854775807/bookings", "a",
		`{"kind":"income","amount_cents":100,"purpose":"p"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("book unknown = %d, want 404", resp.StatusCode)
	}
	// Bad id -> 400.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/foundation-books/abc/bookings", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/foundation-books/abc/balance", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad balance id = %d, want 400", resp.StatusCode)
	}
	// Malformed booking body -> 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/1/bookings", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad booking body = %d, want 400", resp.StatusCode)
	}
}

// TestHTTPFoundationBooksListIsScoped reads one foundation's books over HTTP: the ones created
// for it, and none of another's.
func TestHTTPFoundationBooksListIsScoped(t *testing.T) {
	srv, _ := newTestServer(t)
	mine := createFoundation(t, srv.URL)
	theirs := createFoundation(t, srv.URL)

	myBook := postBook(t, srv.URL, mine.ID)
	theirBook := postBook(t, srv.URL, theirs.ID)

	list := getBooks(t, srv.URL, mine.ID)
	if len(list) != 1 || list[0].ID != myBook.ID || list[0].FoundationID != mine.ID {
		t.Fatalf("mine = %+v, want only %d", list, myBook.ID)
	}
	other := getBooks(t, srv.URL, theirs.ID)
	if len(other) != 1 || other[0].ID != theirBook.ID {
		t.Fatalf("theirs = %+v, want only %d", other, theirBook.ID)
	}

	// a foundation nothing carries has no books, and it is not an error
	if got := getBooks(t, srv.URL, 9223372036854775807); len(got) != 0 {
		t.Fatalf("unknown foundation = %+v, want none", got)
	}
	// a path that is not an id never reaches the service
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/foundations/abc/books", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad books id = %d, want 400", resp.StatusCode)
	}
}

// postBook creates a book for a foundation over HTTP.
func postBook(t *testing.T, srv string, foundationID int64) bookJSON {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv+"/api/foundation-books", "alice",
		`{"foundation_id":`+itoa(foundationID)+`,"reference":"B-`+uniqueSuffix(t)+`","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create book = %d", resp.StatusCode)
	}
	var b bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		t.Fatalf("decode book: %v", err)
	}
	return b
}

// getBooks reads a foundation's books over HTTP, failing the test on anything but 200.
func getBooks(t *testing.T, srv string, foundationID int64) []bookJSON {
	t.Helper()
	resp := doJSON(t, http.MethodGet, srv+"/api/foundations/"+itoa(foundationID)+"/books", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list books = %d", resp.StatusCode)
	}
	var list []bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode books: %v", err)
	}
	return list
}
