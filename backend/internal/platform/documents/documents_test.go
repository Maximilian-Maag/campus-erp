package documents_test

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/golden"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/documents"
)

func fixedNotice() documents.DunningNotice {
	return documents.DunningNotice{
		Reference:   "BK-2026-0001",
		DebtorName:  "Max Mustermann, Musterstadt",
		Issuer:      "Universitaet Musterstadt - Kasse",
		Level:       2,
		AmountCents: 42500,
		DueDate:     "2026-11-15",
	}
}

func TestNoticeHTMLGolden(t *testing.T) {
	got, err := fixedNotice().HTML()
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	golden.Assert(t, filepath.Join("testdata", "golden_notice.html"), []byte(got))
}

func TestNoticeHTMLEscapesAndDefaults(t *testing.T) {
	n := fixedNotice()
	n.DebtorName = `Muster & <Co> "GmbH"`
	got, err := n.HTML()
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if !strings.Contains(got, "Muster &amp; &lt;Co&gt; &#34;GmbH&#34;") {
		t.Fatalf("debtor name must be escaped:\n%s", got)
	}
	if !strings.Contains(got, "425.00 EUR") {
		t.Fatalf("amount/currency missing:\n%s", got)
	}
	n.Currency = "chf"
	got, err = n.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "425.00 CHF") {
		t.Fatalf("currency must upper-case:\n%s", got)
	}
}

func TestNoticeValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*documents.DunningNotice)
		want   error
	}{
		{"reference", func(n *documents.DunningNotice) { n.Reference = " " }, documents.ErrReferenceMissing},
		{"debtor", func(n *documents.DunningNotice) { n.DebtorName = "" }, documents.ErrDebtorMissing},
		{"issuer", func(n *documents.DunningNotice) { n.Issuer = "  " }, documents.ErrIssuerMissing},
		{"level", func(n *documents.DunningNotice) { n.Level = 0 }, documents.ErrLevelInvalid},
		{"amount", func(n *documents.DunningNotice) { n.AmountCents = 0 }, documents.ErrAmountInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := fixedNotice()
			c.mutate(&n)
			if _, err := n.HTML(); !errors.Is(err, c.want) {
				t.Fatalf("HTML error = %v, want %v", err, c.want)
			}
			if err := n.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("Validate error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	got, err := documents.DunningNotice{Reference: "R", DebtorName: "D", Issuer: "I", Level: 1, AmountCents: 5}.HTML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "0.05 EUR") {
		t.Fatalf("minor units must be padded:\n%s", got)
	}
}

func TestNewHTTPRendererValidation(t *testing.T) {
	if _, err := documents.NewHTTPRenderer("  ", nil); !errors.Is(err, documents.ErrURLRequired) {
		t.Fatalf("error = %v, want ErrURLRequired", err)
	}
	r, err := documents.NewHTTPRenderer("http://pdf:3000/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.URL() != "http://pdf:3000/forms/chromium/convert/html" {
		t.Fatalf("url = %q", r.URL())
	}
}

func TestRenderPostsHTMLAndReturnsPDF(t *testing.T) {
	var gotHTML string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/forms/chromium/convert/html" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotContentType = r.Header.Get("Content-Type")
		_, params, err := mime.ParseMediaType(gotContentType)
		if err != nil {
			t.Errorf("content type: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("part: %v", err)
				break
			}
			if part.FormName() == "files" && part.FileName() == "index.html" {
				b, _ := io.ReadAll(part)
				gotHTML = string(b)
			}
		}
		w.Write([]byte("%PDF-1.7"))
	}))
	defer srv.Close()

	r, err := documents.NewHTTPRenderer(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := r.Render(context.Background(), "<html>hallo</html>")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(pdf) != "%PDF-1.7" {
		t.Fatalf("pdf = %q", pdf)
	}
	if gotHTML != "<html>hallo</html>" {
		t.Fatalf("service received %q", gotHTML)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("content type = %q", gotContentType)
	}
}

func TestRenderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	r, err := documents.NewHTTPRenderer(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Render(context.Background(), "<html/>"); !errors.Is(err, documents.ErrRenderFailed) {
		t.Fatalf("error = %v, want ErrRenderFailed", err)
	}
	if _, err := r.Render(context.Background(), "   "); !errors.Is(err, documents.ErrHTMLRequired) {
		t.Fatalf("error = %v, want ErrHTMLRequired", err)
	}
}
