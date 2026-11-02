# 0009 — Render documents with an HTML→PDF service

Status: accepted (2026-11-02)

## Context

Dunning notices, certificates and reports must be printable. The wording belongs to
the domain that owns it, and the backend should not embed a PDF engine.

## Decision

Render the HTML in the application and convert it to PDF with **one additional deploy
component**: a stateless HTML→PDF service. The application posts the HTML it rendered,
stores the returned PDF in the object store, and the case keeps a reference to the
artefact. The component is gated in the chart (off by default).

## Consequences

- Layouts change without a backend release; the HTML is deterministic and testable.
- No PDF engine in the Go binary, and the service is optional.
- One more container to operate. When it is unavailable, document production fails and
  must be retried — the flow must not block the business transaction that triggered it.
- Considered and rejected: a pure-Go PDF library in-process (no extra component, but
  far less control over layout and typography).
