package httpapi_test

import (
	"encoding/json"
	"fmt"
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

// createBookFor opens a book for a foundation over HTTP and returns it.
func createBookFor(t *testing.T, srv string, foundationID int64, reference string) bookJSON {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv+"/api/foundation-books", "alice",
		`{"foundation_id":`+itoa(foundationID)+`,"reference":"`+reference+`","currency":"EUR"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create book = %d", resp.StatusCode)
	}
	var book bookJSON
	if err := json.NewDecoder(resp.Body).Decode(&book); err != nil {
		t.Fatalf("decode book: %v", err)
	}
	return book
}

// TestHTTPFoundationCarryOver closes a year over HTTP: the answer names both books and the net, and
// the two refusals a caller can act on come back as 422 and 409 rather than 500.
func TestHTTPFoundationCarryOver(t *testing.T) {
	srv, _ := newTestServer(t)
	number := "STIFT-" + uniqueSuffix(t)
	// The source year carries; the next year's record is opened first, as an operator would.
	for _, year := range []int{2026, 2027} {
		body := `{"year":` + itoa(int64(year)) + `,"number":"` + number + `","description":"Stiftung","carry_over":true}`
		if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundations", "alice", body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("foundation %d = %d", year, resp.StatusCode)
		}
	}
	var f foundationJSON
	resp := doJSON(t, http.MethodGet, srv.URL+"/api/foundations?year=2026", "alice", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list foundations = %d", resp.StatusCode)
	}
	var list []foundationJSON
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode foundations: %v", err)
	}
	for _, cand := range list {
		if cand.Number == number && cand.Year == 2026 {
			f = cand
		}
	}
	if f.ID == 0 {
		t.Fatalf("foundation %s/2026 not found in %+v", number, list)
	}

	book := createBookFor(t, srv.URL, f.ID, "JAHR-"+uniqueSuffix(t))
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/bookings", "alice",
		`{"kind":"income","amount_cents":9000,"purpose":"Spende"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("income = %d", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/carry-over", "alice", `{}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("carry-over = %d, want 201", resp.StatusCode)
	}
	var out struct {
		SourceBookID    int64  `json:"source_book_id"`
		SourceReference string `json:"source_reference"`
		Year            int    `json:"year"`
		TargetBookID    int64  `json:"target_book_id"`
		TargetReference string `json:"target_reference"`
		BookingID       int64  `json:"booking_id"`
		Cents           int64  `json:"cents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode carry-over: %v", err)
	}
	if out.Cents != 9000 || out.Year != 2026 || out.SourceBookID != book.ID {
		t.Fatalf("carry-over = %+v, want 9000 cents from 2026 book %d", out, book.ID)
	}
	if out.TargetBookID == 0 || out.TargetBookID == book.ID || out.BookingID == 0 {
		t.Fatalf("carry-over = %+v, want a distinct target book and an opening movement", out)
	}

	// The same year again: the money is already in the next book.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/carry-over", "alice", `{}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second carry-over = %d, want 409", resp.StatusCode)
	}

	// A foundation that does not carry: 422, not 500.
	plain := createFoundation(t, srv.URL)
	plainBook := createBookFor(t, srv.URL, plain.ID, "JAHR-"+uniqueSuffix(t))
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(plainBook.ID)+"/bookings", "alice",
		`{"kind":"income","amount_cents":100,"purpose":"Spende"}`); resp.StatusCode != http.StatusCreated {
		t.Fatalf("plain income = %d", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(plainBook.ID)+"/carry-over", "alice", `{}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("carry-over without the flag = %d, want 422", resp.StatusCode)
	}
	// Unknown book: 404. Malformed body: 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/9223372036854775807/carry-over", "alice", `{}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown book = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/carry-over", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
}

// TestHTTPFoundationBookingReversal cancels a movement over HTTP: the answer names both rows, and
// the refusals come back as statuses a client can act on rather than one generic failure.
func TestHTTPFoundationBookingReversal(t *testing.T) {
	srv, _ := newTestServer(t)
	f := createFoundation(t, srv.URL)
	book := createBookFor(t, srv.URL, f.ID, "STORNO-"+uniqueSuffix(t))

	resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/bookings", "alice",
		`{"kind":"income","amount_cents":7500,"purpose":"Spende (doppelt)"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("book a movement = %d", resp.StatusCode)
	}
	var original struct{ ID int64 }
	if err := json.NewDecoder(resp.Body).Decode(&original); err != nil {
		t.Fatalf("decode booking: %v", err)
	}

	resp = doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/"+itoa(original.ID)+"/reverse", "alice",
		`{"reason":"doppelt erfasst"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reverse = %d, want 201", resp.StatusCode)
	}
	// The tags matter: `source_book_id`-style names do not map onto a Go field without one.
	var out struct {
		OriginalID       int64  `json:"original_id"`
		BookID           int64  `json:"book_id"`
		CounterBookingID int64  `json:"counter_booking_id"`
		CounterKind      string `json:"counter_kind"`
		AmountCents      int64  `json:"amount_cents"`
		Purpose          string `json:"purpose"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode reversal: %v", err)
	}
	if out.OriginalID != original.ID || out.BookID != book.ID || out.CounterBookingID == 0 {
		t.Fatalf("reversal = %+v, want it to name %d in book %d and the counter-movement", out, original.ID, book.ID)
	}
	if out.CounterKind != "expense" || out.AmountCents != 7500 {
		t.Fatalf("counter-movement = %s %d, want expense 7500", out.CounterKind, out.AmountCents)
	}
	if out.Purpose != fmt.Sprintf("Storno zu Buchung %d: doppelt erfasst", original.ID) {
		t.Fatalf("purpose = %q, want the reason carried into it", out.Purpose)
	}

	// Cancelling the same movement again is a conflict, not a second counter-movement.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/"+itoa(original.ID)+"/reverse", "alice", `{}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second reversal = %d, want 409", resp.StatusCode)
	}
	// Cancelling the counter-movement is refused with a reason a client can show.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/"+itoa(out.CounterBookingID)+"/reverse", "alice", `{}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("reversing the counter-movement = %d, want 422", resp.StatusCode)
	}
	// Unknown movement: 404. Malformed body and bad id: 400.
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/9223372036854775807/reverse", "alice", `{}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown movement = %d, want 404", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/"+itoa(original.ID)+"/reverse", "alice", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/foundation-bookings/abc/reverse", "alice", `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	// The book's balance is derived, so the pair nets to zero without either row being removed.
	resp = doJSON(t, http.MethodGet, srv.URL+"/api/foundation-books/"+itoa(book.ID)+"/balance", "alice", "")
	var bal balanceJSON
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if bal.IncomeCents-bal.ExpenseCents != 0 {
		t.Fatalf("balance = %d/%d, want it netted to zero", bal.IncomeCents, bal.ExpenseCents)
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
