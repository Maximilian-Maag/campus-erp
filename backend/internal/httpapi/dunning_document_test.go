package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// TestDunningNoticeDocumentRoute covers the stored-document route end to end: a notice
// with no document is a 404, one whose artefact is in the store is served as a PDF, and
// a bad or unknown id is refused.
func TestDunningNoticeDocumentRoute(t *testing.T) {
	ctx := context.Background()
	srv, pool := newTestServer(t)
	repo := kasse.NewDunningRepo(pool)

	notice, err := repo.InsertNotice(ctx, kasse.DunningNotice{
		ClaimReference:   "CLAIM-DOC-" + itoa(time.Now().UnixNano()),
		LevelCode:        "2",
		OutstandingCents: 125000,
		ChargeCents:      500,
		IssuedOn:         time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		IssuedBy:         "setup",
	})
	if err != nil {
		t.Fatalf("InsertNotice: %v", err)
	}
	url := srv.URL + "/api/dunning/notices/" + itoa(notice.ID) + "/document"

	// No document recorded yet: the notice exists, the artefact does not.
	if resp := doJSON(t, http.MethodGet, url, "carol", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no document = %d, want 404", resp.StatusCode)
	}

	// Record the key and put the bytes where the store looks.
	key := "dunning/" + itoa(notice.ID) + ".pdf"
	if err := repo.SetNoticeDocument(ctx, notice.ID, key); err != nil {
		t.Fatalf("SetNoticeDocument: %v", err)
	}
	full := filepath.Join(artefactsDir(t), key)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("%PDF-1.7 test"), 0o640); err != nil {
		t.Fatalf("write artefact: %v", err)
	}

	resp := doJSON(t, http.MethodGet, url, "carol", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("document = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type = %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Fatalf("body = %q", body)
	}

	// A recorded key whose artefact is missing is a server-side inconsistency: the
	// notice points at nothing the store can serve.
	orphan, err := repo.InsertNotice(ctx, kasse.DunningNotice{
		ClaimReference:   "CLAIM-ORPHAN-" + itoa(time.Now().UnixNano()),
		LevelCode:        "2",
		OutstandingCents: 5000,
		ChargeCents:      100,
		IssuedOn:         time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		IssuedBy:         "setup",
	})
	if err != nil {
		t.Fatalf("InsertNotice orphan: %v", err)
	}
	if err := repo.SetNoticeDocument(ctx, orphan.ID, "dunning/never-written.pdf"); err != nil {
		t.Fatalf("SetNoticeDocument orphan: %v", err)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/notices/"+itoa(orphan.ID)+"/document", "carol", ""); resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("missing artefact = %d, want 500", resp.StatusCode)
	}

	// A bad id is a client error; an unknown one is not found.
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/notices/abc/document", "carol", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id = %d, want 400", resp.StatusCode)
	}
	if resp := doJSON(t, http.MethodGet, srv.URL+"/api/dunning/notices/9223372036854775807/document", "carol", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id = %d, want 404", resp.StatusCode)
	}
}
