package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/httpapi"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/db"
)

// testPool opens the tenant database with migrations applied. Tests that only need
// unique values use it directly; newTestServer builds on it.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(db.EnvDatabaseURL)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("DATABASE_URL must be set in CI — e2e tests must not be skipped there")
		}
		t.Skip("DATABASE_URL not set; skipping HTTP e2e test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	return pool
}

func newTestServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	store := audit.NewStore(pool)
	srv := httptest.NewServer(httpapi.New(
		kasse.NewService(kasse.NewRepo(pool), store),
		kasse.NewOrderService(kasse.NewOrderRepo(pool), store),
		kasse.NewDunningService(kasse.NewDunningRepo(pool), store),
		kasse.NewTaxService(kasse.NewTaxRepo(pool), store),
		kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store),
		store,
	).Handler())
	t.Cleanup(srv.Close)
	return srv, pool
}

// testSeqNext returns the next value of a test-only sequence, created on demand.
// The database persists between runs and is shared by test packages that Go runs in
// parallel, so uniqueness must come from the database itself: a timestamp can
// collide and a "find the first free value" scan races.
//
// CREATE SEQUENCE IF NOT EXISTS is not atomic, so parallel packages can race on it;
// the sequence merely existing is all we need, so the error is ignored.
func testSeqNext(t *testing.T) int64 {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "CREATE SEQUENCE IF NOT EXISTS test_unique_seq")
	var n int64
	if err := pool.QueryRow(ctx, "SELECT nextval('test_unique_seq')").Scan(&n); err != nil {
		t.Fatalf("test sequence: %v", err)
	}
	return n
}

// uniqueSuffix is a short, database-unique token (fits the legacy 8-char code
// column) for building codes and numbers in tests.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	return strconv.FormatInt(testSeqNext(t), 36)
}

// uniqueDate returns a database-unique business date. The base year is far beyond
// anything the suite uses, so a freshly created sequence cannot collide with rows
// left behind by earlier runs.
func uniqueDate(t *testing.T) string {
	t.Helper()
	base := time.Date(5000, 1, 1, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, int(testSeqNext(t))).Format(time.DateOnly)
}

func countSubject(t *testing.T, pool *pgxpool.Pool, subject string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_event WHERE subject = $1", subject).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func doJSON(t *testing.T, method, url, actor, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if actor != "" {
		req.Header.Set("X-Actor", actor)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

type dayJSON struct {
	ID           int64 `json:"ID"`
	Status       string
	ClosingCents *int64
}

func openDay(t *testing.T, srv *httptest.Server, date string) dayJSON {
	t.Helper()
	return openDayAs(t, srv, date, "alice")
}

func openDayAs(t *testing.T, srv *httptest.Server, date, actor string) dayJSON {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days", actor,
		`{"business_date":"`+date+`","opening_cents":10000}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("open day = %d", resp.StatusCode)
	}
	var d dayJSON
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		t.Fatalf("decode day: %v", err)
	}
	return d
}

func TestHTTPCashBookFlow(t *testing.T) {
	srv, pool := newTestServer(t)

	if resp := doJSON(t, http.MethodGet, srv.URL+"/healthz", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}

	day := openDay(t, srv, uniqueDate(t))
	base := srv.URL + "/api/cashbook/days/" + itoa(day.ID)

	resp := doJSON(t, http.MethodPost, base+"/bookings", "alice",
		`{"kind":"income","amount_cents":500,"purpose":"Gebühr"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("book = %d", resp.StatusCode)
	}

	// invalid booking -> 422
	if resp := doJSON(t, http.MethodPost, base+"/bookings", "alice",
		`{"kind":"income","amount_cents":0,"purpose":"x"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("zero amount = %d, want 422", resp.StatusCode)
	}

	// unknown day -> 404
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/cashbook/days/9223372036854775807", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown day = %d, want 404", resp.StatusCode)
	}

	// non-integer id -> 400
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/cashbook/days/abc", "", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}

	// close, then booking must conflict
	resp = doJSON(t, http.MethodPost, base+"/close", "bob", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close = %d", resp.StatusCode)
	}
	var closed dayJSON
	if err := json.NewDecoder(resp.Body).Decode(&closed); err != nil {
		t.Fatalf("decode closed: %v", err)
	}
	if closed.Status != "closed" || closed.ClosingCents == nil || *closed.ClosingCents != 10500 {
		t.Fatalf("closed day = %+v, want closed with 10500", closed)
	}
	if resp := doJSON(t, http.MethodPost, base+"/bookings", "alice",
		`{"kind":"income","amount_cents":1,"purpose":"x"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("book on closed day = %d, want 409", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, base+"/close", "bob", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("double close = %d, want 409", resp.StatusCode)
	}

	// audit chain intact: open + book + close = 3 new events
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/audit/verify", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("audit verify = %d", resp.StatusCode)
	}
	subject := fmt.Sprintf("day/%d", day.ID)
	if got := countSubject(t, pool, subject); got != 3 {
		t.Fatalf("audit events for %s = %d, want 3", subject, got)
	}
}

func TestHTTPMalformedBodies(t *testing.T) {
	srv, _ := newTestServer(t)
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad open body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days", "a", `{"business_date":"02.03.2026"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad date = %d, want 400", resp.StatusCode)
	}
	day := openDay(t, srv, uniqueDate(t))
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/"+itoa(day.ID)+"/bookings", "a", `{`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad book body = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/"+itoa(day.ID)+"/bookings", "a",
		`{"kind":"transfer","amount_cents":5,"purpose":"x"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown kind = %d, want 422", resp.StatusCode)
	}
}

// A request without X-Actor must be attributed to "anonymous" (observable in the
// audit stream). This also kills the mutant that inverts the header check.
func TestHTTPActorFallsBackToAnonymous(t *testing.T) {
	srv, pool := newTestServer(t)
	day := openDayAs(t, srv, uniqueDate(t), "")

	var actor string
	if err := pool.QueryRow(context.Background(),
		"SELECT actor FROM audit_event WHERE subject = $1", fmt.Sprintf("day/%d", day.ID)).Scan(&actor); err != nil {
		t.Fatalf("read actor: %v", err)
	}
	if actor != "anonymous" {
		t.Fatalf("actor = %q, want anonymous", actor)
	}
}

// Error branches of every handler.
func TestHTTPErrorBranches(t *testing.T) {
	srv, _ := newTestServer(t)
	date := uniqueDate(t)
	day := openDay(t, srv, date)
	base := srv.URL + "/api/cashbook/days/" + itoa(day.ID)

	// opening the same business date twice -> conflict
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days", "alice",
		`{"business_date":"`+date+`","opening_cents":10000}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate day = %d, want 409", resp.StatusCode)
	}

	// booking on an unknown day -> 404
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/9223372036854775807/bookings", "alice",
		`{"kind":"income","amount_cents":5,"purpose":"x"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("book unknown day = %d, want 404", resp.StatusCode)
	}

	// closing an unknown day -> 404
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/9223372036854775807/close", "bob", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("close unknown day = %d, want 404", resp.StatusCode)
	}

	// blank purpose -> 422
	if resp := doJSON(t, http.MethodPost, base+"/bookings", "alice",
		`{"kind":"income","amount_cents":5,"purpose":"  "}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank purpose = %d, want 422", resp.StatusCode)
	}

	// non-integer id on the mutation routes -> 400
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/abc/bookings", "alice", `{}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id on book = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodPost, srv.URL+"/api/cashbook/days/abc/close", "alice", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id on close = %d, want 400", resp.StatusCode)
	}
}
