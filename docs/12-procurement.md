# Procurement — purchase orders

Purchase orders (Bestellungen) with a lifecycle, mirroring the vendor's
`purchase_order` / `purchase_order_line` (`docs/08-reuse-inventory.md` §3). The
point of the slice is the integration: **approving an order commits budget**
through the budget module (`docs/10-budget.md`).

## Model

`purchase_order` (number UNIQUE, supplier, `budget_position_id` FK, currency,
amounts, status, audit stamps) + `purchase_order_line` (description, quantity,
unit price, net, VAT category/rate). Amounts are integer cents, quantities
milli-units, VAT basis points; totals are derived from the lines.

## Lifecycle

```
draft ──approve──► approved ──receive──► received
  │                   │
  └──────cancel───────┴──► cancelled
```

Every transition is guarded: only `draft→approved`, `approved→received` and
`draft|approved→cancelled` are allowed; anything else is `ErrBadTransition`.

## Budget integration

- **approve** posts a `commitment` of the order's **gross** to the order's budget
  position (`reference = "PO:<number>"`). If the position cannot cover it, the
  budget module returns `ErrBudgetExceeded` and the approval fails — the order
  stays a draft.
- **cancel** of an approved order posts a `release` of the same amount.

So a purchase order reserves budget before any invoice arrives, and the
commitment is visible on the budget position's balance.

## Two-way matching

`MatchReceipt(ctx, orderNumber, grossCents, reference, by)` is the receipt an inbound
invoice triggers (the route lives on the invoice side, `POST /api/e-invoices/{id}/match`,
`docs/09-e-invoicing.md`). It finds the order **by number** — the buyer reference (BT-10)
the supplier echoes back is the order number — and requires:

- the order to be **approved** (a draft never committed anything, a received one is already
  matched, a cancelled one is no longer a liability) — anything else is `ErrBadTransition`;
- the invoice's gross to equal the order's (`ErrMatchAmountMismatch` otherwise).

When both hold it converts the commitment into an actual: a `release` of the commitment
plus an `actual` of the same amount, in one step, so the position's available Mittel are
**unchanged by the conversion** instead of being counted twice. Only then does the order
become `received` (`purchase_order.match` on the audit stream). A refusal leaves the order
and the budget exactly as they were.

## API

| method | path |
|---|---|
| `POST` | `/api/purchase-orders` |
| `GET`  | `/api/purchase-orders/{id}` |
| `POST` | `/api/purchase-orders/{id}/approve` |
| `POST` | `/api/purchase-orders/{id}/receive` |
| `POST` | `/api/purchase-orders/{id}/cancel` |

## Not yet done

- Goods-receipt lines (partial receipts). Matching covers the invoice in full; a partial
  receipt would need per-line matching.
