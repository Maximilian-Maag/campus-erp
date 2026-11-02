// Package documents renders printable documents and turns them into PDFs.
//
// The service itself owns no printer: it renders HTML (templates live with the
// domain that owns them) and posts it to the document service, which returns a PDF
// (ADR-0009). The call is stateless and the HTML is deterministic, so the same input
// always yields the same document.
package documents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

// Errors of the document service.
var (
	ErrURLRequired      = errors.New("documents: service URL is required")
	ErrHTMLRequired     = errors.New("documents: HTML must not be empty")
	ErrRenderFailed     = errors.New("documents: the document service refused the render")
	ErrReadFailed       = errors.New("documents: could not read the rendered document")
	ErrReferenceMissing = errors.New("documents: reference is required")
	ErrDebtorMissing    = errors.New("documents: debtor name is required")
	ErrIssuerMissing    = errors.New("documents: issuer is required")
	ErrLevelInvalid     = errors.New("documents: level must be positive")
	ErrAmountInvalid    = errors.New("documents: amount must be positive")
)

// Renderer turns HTML into a PDF.
type Renderer interface {
	Render(ctx context.Context, html string) ([]byte, error)
}

// HTTPRenderer talks to the document service: it posts the HTML as one file field and
// returns the PDF it answers with.
type HTTPRenderer struct {
	url    string
	client *http.Client
}

// NewHTTPRenderer returns a renderer for the service at url. A nil client uses
// http.DefaultClient.
func NewHTTPRenderer(url string, client *http.Client) (*HTTPRenderer, error) {
	if strings.TrimSpace(url) == "" {
		return nil, ErrURLRequired
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRenderer{url: strings.TrimRight(strings.TrimSpace(url), "/") + "/forms/chromium/convert/html", client: client}, nil
}

// URL is the endpoint the renderer posts to (useful for readiness and logs).
func (r *HTTPRenderer) URL() string { return r.url }

// Render posts html and returns the rendered PDF.
func (r *HTTPRenderer) Render(ctx context.Context, html string) ([]byte, error) {
	if strings.TrimSpace(html) == "" {
		return nil, ErrHTMLRequired
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, fmt.Errorf("documents: multipart: %w", err)
	}
	if _, err := io.WriteString(fw, html); err != nil {
		return nil, fmt.Errorf("documents: multipart: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("documents: multipart: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, &body)
	if err != nil {
		return nil, fmt.Errorf("documents: request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("documents: render: %w", err)
	}
	defer resp.Body.Close()
	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadFailed, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrRenderFailed, resp.StatusCode)
	}
	return pdf, nil
}

// noticeTemplate is parsed once: the notice is pure data in, HTML out.
var noticeTemplate = template.Must(template.New("notice").Parse(
	`<!doctype html>
<html lang="de">
<head><meta charset="utf-8"><title>Mahnung {{.Reference}}</title></head>
<body>
<h1>Mahnung {{.Level}}. Stufe</h1>
<p class="issuer">{{.Issuer}}</p>
<p>Für die Buchung {{.Reference}} bitten wir um Ausgleich von
<strong>{{.Amount}} {{.Currency}}</strong> bis zum {{.DueDate}}.</p>
<p>Zahlungsempfänger: {{.DebtorName}}</p>
</body>
</html>
`))

// DunningNotice is a dunning document for one booking.
type DunningNotice struct {
	Reference   string
	DebtorName  string
	Issuer      string
	Level       int
	AmountCents int64
	Currency    string
	DueDate     string
}

// Validate reports the first missing or invalid field.
func (n DunningNotice) Validate() error {
	if strings.TrimSpace(n.Reference) == "" {
		return ErrReferenceMissing
	}
	if strings.TrimSpace(n.DebtorName) == "" {
		return ErrDebtorMissing
	}
	if strings.TrimSpace(n.Issuer) == "" {
		return ErrIssuerMissing
	}
	if n.Level <= 0 {
		return ErrLevelInvalid
	}
	if n.AmountCents <= 0 {
		return ErrAmountInvalid
	}
	return nil
}

func (n DunningNotice) currency() string {
	if strings.TrimSpace(n.Currency) == "" {
		return "EUR"
	}
	return strings.ToUpper(strings.TrimSpace(n.Currency))
}

// HTML renders the notice. The version used for rendering is derived from the level,
// so a notice is labelled with the stage it belongs to.
func (n DunningNotice) HTML() (string, error) {
	if err := n.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	err := noticeTemplate.Execute(&b, struct {
		Reference  string
		DebtorName string
		Issuer     string
		Level      int
		Amount     string
		Currency   string
		DueDate    string
	}{
		Reference:  strings.TrimSpace(n.Reference),
		DebtorName: strings.TrimSpace(n.DebtorName),
		Issuer:     strings.TrimSpace(n.Issuer),
		Level:      n.Level,
		Amount:     formatAmount(n.AmountCents),
		Currency:   n.currency(),
		DueDate:    strings.TrimSpace(n.DueDate),
	})
	if err != nil {
		return "", fmt.Errorf("documents: notice: %w", err)
	}
	return b.String(), nil
}

// formatAmount renders minor units as a decimal string ("125000" -> "1250.00").
// Amounts are validated positive before rendering, so there is no sign to carry.
func formatAmount(cents int64) string {
	return strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
