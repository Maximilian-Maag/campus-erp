# Budget — Mittelbewirtschaftung

The first non-Kasse ERP slice: budget positions with append-only postings and
**Mittelüberwachung** (a posting may not drive the available amount below zero).
Mirrors the productive model `budget_position` / `budget_posting`
(`docs/08-reuse-inventory.md` §3).

## Model

`budget_position` already existed from the S2 payment-order slice (year, chapter,
title, cost centre/carrier) and is referenced by `payment_order`; migration
`0012` adds the allocated amount and the descriptive fields, and adds
`budget_posting`:

| table | role |
|---|---|
| `budget_position` | a title (Haushaltstitel) with `allocated_cents` |
| `budget_posting` | append-only movements: `commitment` / `release` / `actual` |

## Rule

```
committed = Σ commitment − Σ release
available = allocated − committed − Σ actual
```

A `commitment` or `actual` posting is rejected with `ErrBudgetExceeded` when it
would make `available` negative; a `release` never can. The check is a domain
function (`budget.CheckAvailable`) and is exercised by the over-spend test.

## API

| method | path |
|---|---|
| `POST` | `/api/budget/positions` |
| `GET`  | `/api/budget/positions/{id}` (position + balance) |
| `POST` | `/api/budget/positions/{id}/postings` |
| `GET`  | `/api/budget/positions/{id}/balance` |

Every write is audited (`budget_position.create`, `budget_posting.create`).

## Not yet done

- `budget_source` / `budget_center` and the budget import from the vendor model.
- ~~Automatic commitment when a payment order is approved~~ — done since: approving one binds
  its amount on the position it names (`kasse.OrderService.Approve` posts a `commitment` named
  `ANORD:<reference>`), and the binding is refused with `ErrBudgetExceeded` when it exceeds the
  position's available Mittel, so an order cannot be approved beyond its budget. The binding is
  posted **before** the status changes, so a refused approval leaves the order awaiting a
  decision rather than approved without Mittel.
- What follows from that: an order's position must carry Mittel. The order body's
  `budget_position` now takes an `allocated_cents` (docs/15), written when the position is first
  created — naming an existing position does not rewrite its Ansatz, because an order may not
  change the budget it draws on. Referencing an existing position created through the budget
  module (rather than creating one inline) is the follow-up.
