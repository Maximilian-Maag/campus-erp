# 22 — SEPA direct-debit collection run

Drives the pain.008 generator (`docs/20`) from the mandates (`docs/21`): the
collection counterpart of the payment run. A run gathers mandates into one
direct-debit file for one collection date and one sequence type, and stores the
produced file so the run is reproducible.

## Model

`migrations/0018_direct_debit_run.sql`:

| table | role |
|---|---|
| `direct_debit_run` | one produced pain.008 file: `reference` (unique), the creditor `account_id`, `creditor_id` (SEPA Gläubiger-ID), `collection_date`, `sequence_type`, `local_instrument`, `message_id`, `item_count`, `sum_cents`, the stored `file_xml` + `file_hash`, provenance |
| `direct_debit_run_item` | one collection: `mandate_reference` FK, `end_to_end_id`, `amount_cents`, `purpose`; `UNIQUE (run_id, mandate_reference)` |

The creditor side (name, IBAN, BIC, currency) comes from a `cash_account` — the
same account the credit-transfer run debits.

## API

| method | path |
|---|---|
| `POST` | `/api/direct-debit/runs` — create a run (201) |
| `GET` | `/api/direct-debit/runs/{reference}/file` — the stored pain.008 |

Behind `erp-kasse` like the mandate routes. Creating a run is audited
(`collection_run.create`).

## Behaviour

- **Idempotent on the reference**: re-issuing the same reference returns the
  already-produced run and its unchanged file instead of a second one.
- **Fails closed**: an unknown or inactive mandate, a mandate whose
  `sequence_type` differs from the run's, a blank mandate reference or a duplicate
  mandate in one run is rejected before anything is written.
- **Sequence type is per run**: the run declares one sequence type and every
  selected mandate must match it — a `FRST` mandate cannot ride an `RCUR` run
  (correct SEPA behaviour). The per-run `UNIQUE (run_id, mandate_reference)` is a
  second guard behind the service's own dedupe.
- A mandate may be collected in many runs over time (only within one run is it
  unique).

## Not yet done

- **No claim source**: items are supplied explicitly by the caller, like the
  payment orders the credit-transfer run collects. Linking a run to outstanding
  receivables (dunning cases, fees) so it can select them automatically is a
  follow-up.
- **One sequence type per run** (see above). Grouping items by their mandates'
  sequence types into several `PmtInf` blocks in one file is a later refinement.
- **No EBICS transport**: the file is stored and downloadable; sending it over
  EBICS remains `planned` (`docs/14 §4.3`).
- No UI; the generated Angular client already exposes the two operations.
