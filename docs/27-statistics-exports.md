# 27 — Statistics exports (per state)

The statistics offices (Statistische Landesämter) are requirement **IR-4 / FR-P9**: each
federal state wants a file of the period's figures, delivered to its own layout. This
page describes how that is built so that a *layout* is data and the pipeline never moves.

## The problem with a per-state format

There is no single "statistics format". Each office publishes its own column set, its own
column order, its own headings and its own field separator, and each changes them on its own
schedule. The office layouts are not published for a replacement build
(`open-questions.md` Q8). Encoding each state as code would put the most volatile part of the
system — a column list — behind a recompile, a test run and a deploy.

So the layout is **data**.

## Shape

```go
// internal/core/statistics
type Field string // the canonical values an export can carry
const (
    FieldKey     Field = "key"      // the grouping key (here: the booking purpose)
    FieldIncome  Field = "income"
    FieldExpense Field = "expense"
    FieldNet     Field = "net"
    FieldCount   Field = "count"
)

type Column struct{ Field Field; Title string }

type Format struct {
    State      string   // stable key: the registry key and the job argument
    Name       string   // human name, for the register and the docs
    Delimiter  rune     // ';', ',' or tab
    Columns    []Column // THE LAYOUT: which fields, in which order, under which headings
    Title      string   // optional caption line above the header
    Totals     bool     // append a totals row
    TotalLabel string   // the totals row's key cell (empty = "Summe")
}
```

- **`Field` is the only code surface.** Adding a value a state wants that does not exist yet
  is a new `Field` constant plus a case in `value()`. Everything else is data.
- **A `Format` is the layout.** A state's file is an ordered *projection* of the shared fields:
  the columns pick which fields appear, in what order, under what headings, and the delimiter
  says how they are separated.
- **The `Registry` is the seam.** `statistics.Standard()` (in `registry.go`) holds one entry per
  state. The pipeline asks the registry for a state and renders whatever it returns.

`Group` turns a slice of neutral `Entry{Key, Kind, AmountCents}` into one `Record` per key,
ordered by key, so the same entries always produce the same bytes. `Render(format, records)`
writes the file: an optional caption, a header row of column titles, one row per record, and —
where the format asks — a totals row.

## Adding or changing a state

Both are a **registry edit**, never a pipeline change:

1. **Add a state** — append one `Format` to the slice in `Standard()`. Its `State` is the key
   a `statistics-export` job carries.
2. **Change a state** — edit its `Format`: reorder `Columns`, retitle, add or drop a field,
   change `Delimiter`, turn the caption or the totals row on or off.
3. **Add a value** (rare) — add a `Field` constant and a `value()` case, then reference it from
   whichever layouts want it.

A format is validated on construction (`Format.Validate`): a state and a name, a permitted
delimiter, at least one column, and every column naming a *known* field with a non-empty title.
A typo in a layout is therefore a startup failure, not a wrong file. `Standard()` panics on an
invalid seed, which is a compile-time-constant bug by construction.

## The pipeline

- **`statistics-export`** (`internal/platform/jobs/statistics_export.go`) is the worker kind.
  Args are `{State, From, To, Actor}` (period inclusive, `YYYY-MM-DD`). The worker validates the
  seam is set, the state is non-empty, the period parses and the result carries a plain file
  name and non-empty content; it returns any failure so River retries it.
- **The role does the I/O.** `cmd/worker` supplies a `StatisticsExportFunc`: it looks the state
  up in `statistics.Standard()`, reads the period's confirmed bookings, maps them to
  `statistics.Entry` (grouped by their purpose), renders with the state's format and stores the
  file at `statistics/<state>-<from>-<to>.csv` through the artefact store (docs/26, ADR-0011).
  An unknown state is refused with the list of known ones.
- **The file is reachable.** `GET /api/artefacts/statistics/<name>` (role `kasse`) serves it,
  like every other stored artefact (docs/24).

## Triggering an export from the API

The work runs on the worker role, so the API only queues it:

```
POST /api/statistics/exports   (role kasse)
{ "state": "bw", "from": "2026-11-01", "to": "2026-11-30" }
  -> 202 { "job_id": 42 }
```

- The API role holds an **insert-only** River client (`jobs.NewEnqueuer`: no queues and no
  workers, so it cannot work a job). `jobs.Exports` is the seam it hands to the HTTP layer,
  which is why `httpapi` does not import River. With no queue configured the route answers
  `503` rather than pretending the export was queued.
- The request is validated at the edge (`state` required, the dates parsed as `YYYY-MM-DD`)
  so a doomed job is not queued; the worker re-validates, because a job argument can be
  persisted and replayed by hand.
- The other export/render kinds take the same `{id, from, to}` shape and are wired the same
  way: `POST /api/ledger/exports` (`{from, to}`) and `POST /api/reports/renders`
  (`{report_id, from, to}`). `statement-import` and `sepa-export` follow the pattern when their
  triggers are needed.

## The layouts are placeholders

The three seeded layouts (`default`, `bw`, `by`) are **project-defined placeholders**, marked as
such in `registry.go` and named `(Vorlage)`. They exist so the pipeline is exercised end to end
and so the shape of a layout is concrete. They are **not** the offices' real layouts, which are
not published (Q8). Replacing a placeholder is editing its entry; replacing all of them touches
no other file. Two of them differ on purpose (column order, headings, field set, delimiter, the
totals label) to show the projection doing real work.

## Where things live

| Concern | File |
|---|---|
| Fields, `Format`, `Record`, `Group`, `Render` | `internal/core/statistics/statistics.go` |
| The per-state registry | `internal/core/statistics/registry.go` |
| The job kind | `internal/platform/jobs/statistics_export.go` |
| The bookkeeping → rows wiring | `backend/cmd/worker/main.go` |
| Decision | `adr/0012-per-state-statistics-formats-as-data.md` |
