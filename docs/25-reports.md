# 25 — Reports

A report is a **definition plus rows** (ADR-0010). This is the contract the `report-render`
job kind and the print/centre views build on.

## Shape

```go
report.Definition{ID: "kasse-daily-close", Title: "Kassenabschluss", Columns: []report.Column{...}}
report.Report{Definition: ..., Rows: [][]string{...}, GeneratedAt: ...}
```

- **The domain builds it.** `kasse.DailyCloseReport(generatedAt, bookings)` turns a
  period's bookings into the cash book's daily-close report, ending with the totals of
  income, expense and the net.
- **The core validates and renders it.** `Validate` refuses a definition without an id,
  title or columns, duplicate column keys, and any row that does not match the column
  count; `MarshalCSV` and `MarshalHTML` render it. Every interpolated value is escaped by
  `html/template`.
- **Values are strings**, so currency and date formatting happens in the domain, where
  the meaning is known — not in a generic formatter.

## Output

- **CSV** for import into a spreadsheet or the ledger side.
- **HTML** for the document service (ADR-0009), which turns it into a PDF.

## Rendering a report

The `report-render` worker kind (`jobs.ReportRenderArgs{ReportID, From, To, Actor}`) picks a
report by id, collects its data and stores the artefact. The worker role knows
`kasse-daily-close`: it collects the period's bookings, builds the report, renders the HTML,
converts it through the document service (ADR-0009) and stores the PDF under
`reports/<report id>-<from>-<to>.pdf` (ADR-0011). An unknown report id is refused.

## Adding a report

1. Define it: a `report.Definition` with a stable id (the job's argument) and its columns.
2. Build it: a function in the owning domain turning its data into rows.
3. Register it in the worker role's report renderer.

## Not yet done

- More reports: the print/centre views and the statistics exports need their own
  definitions and data builders.
