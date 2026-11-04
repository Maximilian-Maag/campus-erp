# KLR — Kosten- und Leistungsrechnung

Cost accounting: cost centres (Kostenstellen), cost units (Kostenträger) and
append-only accounting records, with a per-cost-centre report and a distribution
engine that moves a centre's costs onto its receivers. Mirrors the vendor's
`cost_center` / `cost_unit` / `accounting_record` (`docs/08` §3); the legacy side
is the `cob` schema (`docs/03` §3).

## Model

| table | role |
|---|---|
| `cost_center` | Kostenstelle (number UNIQUE, name) |
| `cost_unit` | Kostenträger (number UNIQUE, name) |
| `accounting_record` | append-only posting: cost centre, optional cost unit, kind `cost`/`revenue`, amount, business date |
| `allocation` | one distribution key: source centre, receiving centre, share, validity |
| `allocation_run` | one distribution of a centre's period onto its receivers |

Amounts are integer cents. A correction is a compensating record, never an edit.

A record carries a **business date** (the period it belongs to) as well as the
moment it was entered. The two differ in the normal case: the month's costs are
entered after the month ended, and that is the period they must be distributed
over.

## Report

```
costs    = Σ amount where kind = 'cost'
revenues = Σ amount where kind = 'revenue'
balance  = revenues − costs
```

The report is scoped to one cost centre, so one cost centre never sees another's
records (tested).

The same figures reach the report catalogue: `klr-cost-center-overview` (Kostenstellenübersicht,
docs/25) is every cost centre with its Kosten, Erlöse and Saldo and a totals row, rendered through
the same worker as the other reports.

## Distribution (Umlage)

A cost centre's costs are distributed over its receivers by **keys**, which are
data — the vendor's `overhead_calc*` / `allocation*` shape is not published, so a
key is a row (source centre, receiving centre, share in basis points, validity)
and an operator changes a distribution without the engine moving. The reasoning
is the same as for the per-state statistics layouts (ADR-0012).

Running a distribution for a period (`from`..`to`, both inclusive):

1. The keys **in force at the period's end** are the ones that distribute it.
2. Their shares must add up to exactly 10000 basis points. Less is refused rather
   than silently under-distributing; more is refused by the table as well.
3. The source's costs **in that period** (by business date) are split by the
   shares. The split is exact: each receiver gets its share rounded down and the
   **last one takes the remainder**, so no cent is created or lost — a share that
   rounds to nothing books nothing (a record's amount is always positive).
4. Each receiver is charged a `cost`; the source is relieved by the same total
   through a `revenue`. The records carry the run's reference (`UML:<run>`), so a
   distributed cost is traceable in the report.

The source's balance therefore nets to zero and **the sum of balances across the
centres is unchanged** — a distribution moves cost, it neither creates nor
destroys any (tested). The cost *volume* does rise by the allocated amount, since
the receivers' records are new and the source's are not removed: records are
append-only.

One run per cost centre and period, enforced by an exclusion constraint over the
period — **overlapping** windows are refused, not merely identical ones —
because two runs over the same days would hand out the same costs twice. The
records and the run are written in one transaction, so a run is never half
booked, and a refused run leaves the books untouched.

Receivers are **cost centres**. Distributing onto a cost *unit* (Kostenträger) is
a separate step: a record always belongs to a cost centre, so attributing a cost
to a unit goes through the Kostenträgerrechnung, not through this engine, and no
column for it was added here.

## API

| method | path |
|---|---|
| `POST` | `/api/klr/cost-centers` |
| `GET`  | `/api/klr/cost-centers` |
| `POST` | `/api/klr/cost-units` |
| `POST` | `/api/klr/cost-centers/{id}/records` |
| `GET`  | `/api/klr/cost-centers/{id}/report` |
| `POST` | `/api/klr/cost-centers/{id}/allocations` |
| `GET`  | `/api/klr/cost-centers/{id}/allocations` |
| `POST` | `/api/klr/cost-centers/{id}/allocations/run` |

The frontend has one screen for this, *Umlage* (`/klr/umlage`): it picks a cost centre, shows
its keys, defines one (the share is typed as a percentage and stored as basis points), and runs
a period. The rule that a centre's shares must add up to 100 % stays with the API — the screen
does not re-derive it.

A record's `business_date` may be left out, in which case the API takes today —
the edge decides what "today" is, and the domain is always given a date. Every
write is audited (`cost_center.create`, `cost_unit.create`,
`accounting_record.create`, `allocation.define`, `allocation.run`), and
`allocation.run` names the period, the amount distributed and the number of
receivers.

## Not yet done

- `cost_type_bridge` (link FiBu accounts to cost types) and the clearing tables
  (`cost_center_clearing`, `cost_unit_clearing`).
- Distributing onto cost units (Kostenträgerrechnung); the engine refuses a
  period it cannot reach rather than approximating one.
- Internal pricing (`overhead_calc*` rates): the engine distributes actual costs
  by keys, it does not yet price an internal service and charge it out.
