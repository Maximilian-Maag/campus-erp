# 30 - Return postings to FiBu

A returning bank-statement line is money the bank took back: a direct debit the debtor's bank
refused (posting key `009`), a payment-out that came back (`059`). The cash movement is the
*opposite* of the original, so the financial-ledger side needs the counterpart posting that
reverses it. This page describes that file, the service that builds it and the screen that hands it
over.

It is the return-side counterpart of the posting export (`docs/23`): where that export carries the
confirmed bookings of a period, this one carries the movements the bank sent back, and both are
handed over through the same artefact store.

## What it is

`kasse.ReturnPostingsService` collects the period's returning statement lines — those the
reconciliation classified `returned` (`docs/legacy/kbs-statement-import.md`) — and renders one
counterpart posting per line. A line's **effective date** is its value date, falling back to its
booking date when the bank sent none; the period is that date, because that is the day the return
takes effect. A line that carried neither date is refused (`ErrReturnLineUndated`) rather than
dated by guesswork.

The reference of a return posting is the statement line it came from (`RT-<id>`). Re-exporting a
period therefore carries the same references and the ledger can de-duplicate, which is what makes
the export idempotent — the same rule the posting export follows.

## Format (`cash-returns/v1`)

CSV. Three comment lines carry the version, the period and the cash account, then one row per
returning line:

```csv
# cash-returns/v1
# period: 2026-11-01..2026-11-30
# cash_account: 1200
reference,value_date,booking_date,account,cash_account,direction,amount_cents,currency,posting_key,order_reference,purpose
```

- `reference` — the idempotency key (`RT-<statement line id>`).
- `value_date` / `booking_date` — the bank's dates, ISO-8601. A line without a booking date is
  written with an empty field rather than a zero date, so the file never claims a day the
  statement did not name.
- `account` — the counterpart account returns are cleared to (default `1300`); `cash_account` is
  written on every row so a row is self-contained.
- `direction` — the direction of the **counterpart** posting, which is the direction of the cash
  movement away from cash: money into our account debits cash and credits the counterpart
  (`credit`), money out does the reverse (`debit`).
- `amount_cents` — integer minor units; never a float, so there is no rounding drift.
- `posting_key` — the bank's return posting key (`009` / `059`): the evidence of what the bank did,
  kept in the file so the ledger can see it without the statement.
- `order_reference` — the order the returned movement belonged to, when the line reconciled to one,
  so the return can be tied back to its original posting.

`ledger.ReturnExport.Marshal` sorts by value date and then reference and trims its fields, so the
same line set always produces byte-identical output, whatever order it was collected in. Values
containing commas or quotes are quoted by `encoding/csv`.

## Collection

`ReturnPostingsService.Export` validates its actor and its accounts (default `cash 1200` /
`return 1300`), refuses an inverted period, and refuses a period in which nothing came back
(`ErrNoReturnedLines`) rather than handing over an empty file. It validates the rendered export
before it records anything, so a run that could not render is not recorded as one that happened,
and it writes one `ledger.return-postings` audit event with the counts.

## Handing the file over

The `return-postings` worker kind (`jobs.ReturnPostingsArgs{From, To, Actor}`) runs the export and
writes the file through the **artefact store** (ADR-0011, `docs/26`) under
`ledger/return-postings-<timestamp>.csv`, next to the posting export it reverses. `POST
/api/ledger/return-postings` (role `kasse`) queues one for a period: `{from, to}` → `202` with the
job id (`docs/27` describes the queueing seam).

## The read the screen uses

`GET /api/bank-statement-returns?from=&to=` (role `kasse`) lists the returning lines of a period,
each with its reconciliation outcome — the same set the export is built from, so the screen shows
what the file will carry. The period is required and belongs to the server: an empty answer means
"none came back", and a period it will not read is a refusal (`422`), so the two never look alike.

## Screen — *Kasse: Rückläufer* (`/kasse/ruecklaeufer`)

States a period and lists the returning lines: value date, posting key, order reference, amount,
direction (eingehend / ausgehend) and purpose. A loaded period offers the hand-over, which queues
the file and reports the job it was queued as; an empty period offers none, because there is
nothing to hand over. The read and the queued export are asked for the *same* period — the export
follows the fields, not the rows on screen.

## Not yet done

- **Delivery.** The file is written through the artefact store, so only the *hand-over* is missing —
  the same SFTP/bank-channel delivery `docs/23` is waiting for.
- **The vendor's own `rueckfibu_*` layout.** The legacy tables hold the return postings, but the
  format FiBu reads is not published (open question Q8). The layout here is therefore the target
  system's own — versioned and deterministic — and a real per-vendor layout replaces it without a
  code change, the way a statistics layout does (`docs/27`).
- **The reversal itself.** Producing the file is this slice; booking the reversal against the
  original posting is the ledger's (Phase 4).
