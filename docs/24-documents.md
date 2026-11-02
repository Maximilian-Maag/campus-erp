# 24 — Documents

Printable documents (dunning notices first, then certificates and reports) are rendered
as HTML and converted to PDF by one dedicated component (ADR-0009).

## Shape

- **Templates live with the domain** that owns the wording
  (`internal/platform/documents`: `DunningNotice.HTML`), so a change to the text is a
  change to that package's tests, not to a deployment.
- **`documents.Renderer`** turns HTML into a PDF. `NewHTTPRenderer` posts the HTML to
  the document service and returns its answer; the request is stateless.
- **The artefact** goes to the object store, and the case/booking keeps a reference to
  it. Production must not block the business transaction that triggered it: a failure
  is retried, not fatal.
- **Nothing is embedded**: the backend carries no PDF engine; the service is gated in
  the chart (`pdf.enabled`, off by default).

## Conventions

- The HTML is deterministic and golden-tested, and every interpolated value is escaped
  by `html/template`.
- Amounts are integer minor units formatted at render time, so no float rounding
  reaches a notice.

## Fetching a stored document

`GET /api/dunning/notices/{id}/document` serves a notice's stored PDF (role `kasse`). The
notice row carries the artefact key; the bytes come from the artefact store, so the route
works the same whether that is a directory or an S3 bucket (ADR-0011). A notice with no
document is a 404, a malformed id a 400, and a recorded key whose artefact cannot be read a
500 - a server-side inconsistency rather than a client error.

## Not yet done

- Artefacts are served by `GET /api/artefacts/{kind}/{name}` (role `kasse`), where `<kind>`
  is one of `dunning`, `reports`, `ledger`, `sepa`. Two path segments rather than a wildcard:
  the Go router and the contract generator both handle it, and an allowlist of kinds is what
  keeps a download route from becoming a read-anything primitive over the store. The kind
  fixes the media type, the extension overrides it where a report is not a PDF (a CSV
  ledger export is `text/csv`), and anything unrecognised is `application/octet-stream`.

## Wiring the dunning run

`DunningService.WithDocuments` takes a `kasse.DunningDocuments`: issuing a notice renders
the HTML, converts it to PDF through the document service and stores the bytes under
`dunning/<notice id>.pdf`. The key goes into `dunning_notice.document_key` and into the
`dunning.notice.issue` audit detail, so the artefact and the case are tied together.

A render or store failure fails the issue: a notice must not claim a document it does not
have. When `ERP_DOCUMENTS_URL` or `ERP_DOCUMENTS_DIR` is unset the service runs without a
document issuer and issues notices with no artefact — the existing behaviour.

The store is a seam: `documents.Store` is implemented by `DirStore` for local use, and
object storage replaces it without touching the dunning workflow.
