# KLR — Kosten- und Leistungsrechnung

Cost accounting: cost centres (Kostenstellen), cost units (Kostenträger) and
append-only accounting records, with a per-cost-centre report. Mirrors the
vendor's `cost_center` / `cost_unit` / `accounting_record` (`docs/08` §3); the
legacy side is the `cob` schema (`docs/03` §3).

## Model

| table | role |
|---|---|
| `cost_center` | Kostenstelle (number UNIQUE, name) |
| `cost_unit` | Kostenträger (number UNIQUE, name) |
| `accounting_record` | append-only posting: cost centre, optional cost unit, kind `cost`/`revenue`, amount |

Amounts are integer cents. A correction is a compensating record, never an edit.

## Report

```
costs    = Σ amount where kind = 'cost'
revenues = Σ amount where kind = 'revenue'
balance  = revenues − costs
```

The report is scoped to one cost centre, so one cost centre never sees another's
records (tested).

## API

| method | path |
|---|---|
| `POST` | `/api/klr/cost-centers` |
| `POST` | `/api/klr/cost-units` |
| `POST` | `/api/klr/cost-centers/{id}/records` |
| `GET`  | `/api/klr/cost-centers/{id}/report` |

Every write is audited (`cost_center.create`, `cost_unit.create`,
`accounting_record.create`).

## Not yet done

- `cost_type_bridge` (link FiBu accounts to cost types) and the clearing tables
  (`cost_center_clearing`, `cost_unit_clearing`).
- Overhead allocation / internal pricing (`overhead_calc*`, `allocation*`) — the
  distribution engine from `docs/08` §3.
