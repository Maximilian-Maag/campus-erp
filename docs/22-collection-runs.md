# 22 — SEPA direct-debit collection run

Drives the pain.008 generator (`docs/20`) from the mandates (`docs/21`): the
collection counterpart of the payment run. A run gathers mandates into one
direct-debit file for one collection date, and stores the produced file so the run
is reproducible. It is filed either under **one** sequence type — every mandate must
match it — or under **none**, in which case each mandate rides its own type and the
file carries one payment instruction (`PmtInf`) per type.

## Model

`migrations/0018_direct_debit_run.sql` and `migrations/0024_receivable.sql`:

| table | role |
|---|---|
| `direct_debit_run` | one produced pain.008 file: `reference` (unique), the creditor `account_id`, `creditor_id` (SEPA Gläubiger-ID), `collection_date`, `sequence_type`, `local_instrument`, `message_id`, `item_count`, `sum_cents`, the stored `file_xml` + `file_hash`, provenance |
| `direct_debit_run_item` | one collection: `mandate_reference` FK, `end_to_end_id`, `amount_cents`, `purpose`; `UNIQUE (run_id, mandate_reference)` |
| `receivable` | one open item (offener Posten): `mandate_reference` FK, `amount_cents`, `currency`, `due_date`, `purpose`, soft `state` (`open`/`collected`/`cancelled`), `collected_run_id` FK, `source`, provenance |

A receivable carries no debtor of its own: the mandate it names supplies the
debtor and the authority to collect. Its `state` is soft because a collected item
is what a run took, not something to delete (ADR-0006) — an item is linked to the
run that collected it, never removed.

The creditor side (name, IBAN, BIC, currency) comes from a `cash_account` — the
same account the credit-transfer run debits.

## API

| method | path |
|---|---|
| `POST` | `/api/direct-debit/runs` — create a run from explicit items (201) |
| `POST` | `/api/direct-debit/runs/from-receivables` — create a run that selects its open items |
| `GET` | `/api/direct-debit/runs/{reference}/file` — the stored pain.008 |
| `POST` | `/api/receivables` — record an open item (201) |
| `GET` | `/api/receivables` — list open items by due date, `?state=` to narrow |
| `GET` | `/api/receivables/{id}` — read one open item |

Behind `erp-kasse` like the mandate routes. Creating a run and recording an item
are audited (`collection_run.create`, `receivable.create`).

*Kasse: Einzüge* (`/kasse/einzuege`) offers both routes: it gathers the open
receivables due by a date, or names the mandates and amounts by hand. The hand-picked
flow loads the mandates the run's sequence type can collect under (the list route of
`docs/21`), so a choice the run would refuse is not offered — changing the sequence
type reloads them and drops a chosen item whose mandate is no longer collectable.

## Behaviour

- **Idempotent on the reference**: re-issuing the same reference returns the
  already-produced run and its unchanged file instead of a second one — for the
  selecting run too, where the items are by then already collected.
- **Fails closed**: an unknown or inactive mandate, a mandate whose
  `sequence_type` differs from the run's, a blank mandate reference or a duplicate
  mandate in one run is rejected before anything is written.
- **The sequence type belongs to the payment instruction, not to the message.** A run
  declares one type and every selected mandate must match it — a `FRST` mandate cannot
  ride an `RCUR` run (correct SEPA behaviour). Filed with **no** type, the run takes
  each mandate's own and writes one `PmtInf` per type, in the SEPA lifecycle order
  (`FRST`, `RCUR`, `OOFF`, `FNAL`) rather than in the order the items arrived; the
  group header then totals the instructions it carries. Such a run records an empty
  sequence type, which the API and the screen show as *gemischt*. A run of one type is
  unchanged: one instruction, identified by the run's reference, so a file already
  handed to a bank keeps its shape.
- The per-run `UNIQUE (run_id, mandate_reference)` is a second guard behind the
  service's own dedupe within a type.
- A mandate may be collected in many runs over time (only within one run is it
  unique).

## Selecting from open items

`POST /api/direct-debit/runs/from-receivables` takes the run's fields plus a
`due_by` date and chooses its items itself, rather than being handed them:

1. The open items **due on or before `due_by`** are selected, under an **active
   mandate whose sequence type is the run's** — an item that is not yet due, whose
   mandate is retired, or whose mandate collects under a different sequence cannot
   be gathered and is left open, not failed on.
2. A mandate's items are collected as **one amount**: the pain.008 file allows one
   collection per mandate, so several open items under one mandate are summed and
   their purposes joined (distinct, capped at the ISO 140-character remittance
   limit). The end-to-end id is the mandate reference, unique within the run.
3. The selected items are marked `collected` and pointed at the run **in the same
   transaction** that stores it, so a stored run and its claims cannot disagree and
   a run that fails to claim every item it selected is rolled back whole rather
   than half booked.
4. The run's currency is the account's; an open item in another currency is
   refused (`ErrReceivableCurrencyMismatch`) rather than collected at a silent
   rate of one.

`due_by` must not be after the collection date: an item cannot be collected before
it is due. Selecting nothing is `ErrNoCollectionItems`, not an empty file.

## Not yet done

- ~~**One sequence type per run.**~~ — done since: a run filed without a sequence type
  collects each mandate under its own and the file carries one `PmtInf` per type
  (`migrations/0030_collection_run_mixed.sql` widens the run's check to allow "no single
  type").
- **No EBICS transport**: the file is stored and downloadable; sending it over
  EBICS remains `planned` (`docs/14 §4.3`).

