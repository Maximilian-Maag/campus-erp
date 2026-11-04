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

## The catalogue

`internal/platform/reports` holds the list of reports this system renders — the definition
plus the collector that fills it — so the worker that renders a report and the API that offers
it read **one** list and cannot disagree about what exists. `GET /api/reports` (role `kasse`)
returns those definitions, columns included, so a client offers the reports that actually
render instead of a hardcoded list.

| id | what it is |
|---|---|
| `kasse-daily-close` | Kassenabschluss: a period's bookings, closing with income, expense and net |
| `budget-position-overview` | Mittelübersicht: every position with Ansatz, Verpflichtet, Ist and Verfügbar, plus totals |
| `klr-cost-center-overview` | Kostenstellenübersicht: every cost centre with its Kosten, Erlöse and Saldo, plus totals |
| `klr-cost-unit-overview` | Kostenträgerübersicht: every cost unit with its Kosten, Erlöse and Saldo, plus totals |

The Mittelübersicht and the two KLR overviews are snapshots, so they take no period: they
read every position (or centre, or unit) and the balance booked against each. The Mittelübersicht's
available column is derived per position from the same balance the guard uses, so the report
cannot disagree with what the budget will allow; the KLR overviews' balances are derived
the same way from the figures their rows show, so the totals cannot disagree with the rows above
them.

The two KLR overviews are the two stages of the KLR: the Kostenstellen say where a cost arose,
the Kostenträger say what it was for. A cost unit nothing is booked on is still a row of zeroes —
"nothing is booked on it yet" is a fact about the unit, and leaving it out would read as if the
unit did not exist. A record that names no cost unit belongs to no unit and is counted in neither.

## Rendering a report

The `report-render` worker kind (`jobs.ReportRenderArgs{ReportID, From, To, Actor}`) asks the
catalogue for the report, renders the HTML, converts it through the document service
(ADR-0009) and stores the PDF under `reports/<report id>-<from>-<to>.pdf` (ADR-0011). An
unknown report id is refused — the catalogue is the authority on what exists — and so is a
collector whose data source was not wired, rather than storing an empty report that looks
like a real one.

`POST /api/reports/renders` (role `kasse`) queues one: `{report_id, from, to}` -> `202`
with the job id, and the PDF is then reachable through `GET /api/artefacts/reports/<name>`
(docs/27 describes the queueing seam).

## Adding a report

1. Define it: a `report.Definition` with a stable id (the job's argument) and its columns.
2. Build it: a function in the owning domain turning its data into rows.
3. Register it in `internal/platform/reports` — the one list — with the collector that reads
   its data. The API then offers it and the worker renders it, without either being told twice.

## Not yet done

- More of the print views: the Mittelübersicht and the Kostenstellenübersicht are the centre
  views; the print layouts the vendor ships are still to come.
- The statistics exports are their own mechanism (a per-state layout over aggregated figures),
  not a report definition — see docs/27 and ADR-0012.
