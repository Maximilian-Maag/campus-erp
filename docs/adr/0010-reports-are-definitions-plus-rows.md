# 0010 — A report is a definition plus rows

Status: accepted (2026-11-02)

## Context

Reports were named as a worker job kind, but nothing defined what a report *is*. Without
that, a renderer would have to guess its input, and every report would invent its own
output format.

## Decision

A report is **data, not code paths**: a `Definition` (id, title, ordered columns with a
key, a heading and an alignment) plus **rows of strings**, one value per column. The
domain that owns the data builds the report (`kasse.DailyCloseReport`); `internal/core/report`
owns the shape, the validation and the two output formats — CSV and printable HTML.

Rendering to PDF is the document service's job (ADR-0009): the report is rendered to HTML
and handed to the same service dunning notices use.

## Consequences

- One shape for every report: a new report is a definition plus a builder, reviewed as
  data — no new renderer, no new format.
- Rows are strings, so formatting (currency, dates) is decided by the domain at build
  time, where the meaning is known.
- Report identity is a stable id (`kasse-daily-close`), which is what the `report-render`
  job kind takes as its argument.
- Considered and rejected: a report as a hand-written template per report (no shared
  validation, no shared formats, testability by screenshot only).
