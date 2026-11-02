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

## Not yet done

- Wiring the dunning run to render a notice per case, store it and record the
  reference (issue #39); the notice + renderer themselves are in place.
- The service URL is not read from configuration yet (`ERP_DOCUMENTS_URL`).
