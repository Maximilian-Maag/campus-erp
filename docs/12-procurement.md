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
- the invoice's gross to be **positive** (`ErrMatchAmountNotPositive`) and **no more than the
  order's gross** (`ErrMatchAmountMismatch` otherwise). It need not be the whole order: an invoice
  that bills one delivery is matched on its own amount (see *Billing in parts* below).

When both hold it converts the commitment into an actual: a `release` of the commitment
plus an `actual` of the same amount, in one step, so the position's available Mittel are
**unchanged by the conversion** instead of being counted twice. Only then does the order
become `received` (`purchase_order.match` on the audit stream). A refusal leaves the order
and the budget exactly as they were.

## Goods receipts (Wareneingang)

A delivery arrives line by line, in parts, and often before any invoice — the whole-order match
cannot express that, because an inbound invoice states one amount for one order.
`goods_receipt` / `goods_receipt_line` record the **quantities** that arrived per order line,
append-only; the received quantity of an order line is derived from them rather than stored on the
line, so the two cannot disagree.

`RecordReceipt(ctx, orderID, reference, receivedOn, lines, by)` records one delivery:

- the order must be **approved** — a draft committed nothing, a received order has arrived in
  full, a cancelled one is no longer a liability — anything else is `ErrBadTransition`;
- the delivery note is the idempotency key (`goods_receipt_key` is unique per order): recording
  the same note twice is `ErrReceiptExists` rather than a second receipt;
- per line the quantity must be positive (`ErrReceiptQuantity`), must name a line the order
  carries (`ErrReceiptLineUnknown`), may not appear twice (`ErrReceiptLineDuplicate`), and may not
  exceed what is still open on that line (`ErrOverReceipt` — a delivery of more than was ordered is
  a different transaction, not a silent clamp);
- the value it converts is the price of the quantities it carries (net at the line's unit price
  plus the line's own VAT) — **except** on the receipt that completes the order, which converts the
  remaining commitment instead. Rounding a group's VAT once (the order, `ComputeTotals`) and a
  part's VAT per part cannot agree in general, so the residual belongs to the last part; that is
  what makes the receipts of an order add up to its gross **to the cent**;
- the conversion is a `release` plus an `actual` of that value in one step, so the position's
  available Mittel are **unchanged** by it. A delivery of goods that cost nothing (a unit price of
  zero) is recorded without a budget movement, because there is nothing to convert;
- the order becomes `received` once every line has arrived in full;
- one `purchase_order.receipt` audit event carries the delivery note, the value and the state.

The order's state is `ReceiptState`: every line's ordered / received / open quantity, whether the
order is complete, and the receipts recorded so far. A line that nothing arrived of is wholly open,
so a state with no receipt at all is the same shape as the tenth delivery rather than a special
case.

**Why the invoice match converts the remainder.** `MatchReceipt` converts what is *left* of the
commitment (the order's gross less what its receipts and earlier invoices already converted) rather
than the whole gross. That is what lets a delivery and an invoice coexist: an order that was partly
delivered and then invoiced in full consumes its gross exactly once instead of twice, and an order
whose commitment is fully converted is no longer matchable.

## Billing in parts

An order may be billed in more than one invoice. The order records how much of its commitment has
been invoiced (`purchase_order.invoiced_cents`), so each invoice is checked against what is left
rather than against the gross again:

- an invoice **below** what is left converts its own amount and leaves the order `approved` — the
  rest is still expected;
- an invoice that **completes** what is left converts the remainder and closes the order
  (`received`);
- an invoice **above the order's gross** is refused, and so is one that states nothing;
- an invoice after the order is fully converted is refused (`ErrBadTransition`): there is nothing
  left to convert.

The running total is a column rather than a sum over the invoices that matched, because those live
in the e-invoice module and procurement must not read another module's tables to know its own state.
Receipts keep their own total, and the two together may never exceed the gross: a delivery recorded
after a partial invoice converts only what is left, so the commitment is never consumed twice.

*Bestellungen* (`/procurement/bestellungen`) shows the invoiced amount per order — `—` for nothing
billed, `5.000,00 von 11.900,00` for a part, `11.900,00 (vollständig)` for all of it — because
"approved" alone no longer says whether money is still expected.

## API

| method | path |
|---|---|
| `GET`  | `/api/purchase-orders` (optional `?status=`) |
| `POST` | `/api/purchase-orders` |
| `GET`  | `/api/purchase-orders/{id}` |
| `POST` | `/api/purchase-orders/{id}/approve` |
| `POST` | `/api/purchase-orders/{id}/receive` |
| `POST` | `/api/purchase-orders/{id}/cancel` |
| `POST` | `/api/purchase-orders/{id}/receipts` (a delivery; `201`, the receipt and the state) |
| `GET`  | `/api/purchase-orders/{id}/receipts` (the order's receipt state) |

Every write is audited (`purchase_order.receipt` as well as the lifecycle events above).

## Screen — *Beschaffung: Wareneingang* (`/procurement/wareneingang`)

Picks one of the approved orders (the picker is the server's own `?status=approved` filter), lists
every line's ordered, received and open quantity as the server derived it, and books what arrived.
A quantity is typed in units and sent in milli-units (`quantityToMilli`), like an amount typed in
euro and sent in cents; only the lines that are still open get an input, and a blank input means
"nothing of this line", so a delivery of one line of a three-line order is one number rather than
three. An order that arrived in full says so and offers no further booking, and after a booking the
picker is read again because a completed order leaves the approved list.

## Not yet done

- ~~Goods-receipt lines (partial receipts)~~ — done since: `goods_receipt` records what arrived per
  line, the order is complete when every line has arrived, and the invoice match converts the
  remainder (`docs/12`, above).
- ~~What remains on the invoice side: an inbound invoice is still matched against the order's gross
  in full.~~ — done since: an order may be billed in parts (`purchase_order.invoiced_cents`), each
  invoice converts what it bills, and the order closes only when nothing is left (see *Billing in
  parts*).
