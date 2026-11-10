# 23 - Ledger posting export

Confirmed cash bookings must reach the financial-ledger side. The target system
defines that interface itself (it is not inherited from the system being replaced), so
the format is ours: versioned, deterministic and idempotent.

## Format (`cash-postings/v1`)

CSV. Three comment lines carry the version, the period and the cash account, then one
row per posting:

```csv
# cash-postings/v1
# period: 2026-10-01..2026-10-31
# cash_account: 1200
reference,booking_date,account,cash_account,direction,amount_cents,currency,purpose
```

- `reference` - the stable booking reference. Re-exporting a booking re-uses it, which
  is what makes the export idempotent and lets a consumer de-duplicate.
- `direction` - the direction of the **counterpart** posting: an income credits the
  counterpart and debits cash; an expense does the opposite. `cash_account` is written
  on every row so a row is self-contained.
- `amount_cents` - integer minor units; never a float, so there is no rounding drift.
- `currency` - ISO 4217, defaulting to `EUR`.

`ledger.Export.Marshal` sorts by booking date and then reference and trims the fields,
so the same booking set always produces byte-identical output, whatever order it was
collected in. Values containing commas or quotes are quoted by `encoding/csv`.

## Collection

`kasse.LedgerExportService.Export` collects the confirmed bookings of a period
(`LedgerRepo.ListBookingsForExport`, ordered by business date and booking id), maps each
to a posting and renders the file. Income credits the counterpart account and an
expense debits it; the accounts come from `LedgerAccounts` (defaults 1200 / 8400 /
4400). A blank actor, incomplete accounts, an inverted period or an empty period are
refused rather than silently producing an empty file, and every run is recorded on the
audit stream.

## Handing the file over

The `ledger-export` worker kind (`jobs.LedgerExportArgs{From, To, Actor}`) runs the
export and writes the file through a `LedgerExportFunc`. The worker role's implementation
stores it through the **artefact store** (ADR-0011, docs/26) under
`ledger/ledger-postings-<timestamp>.csv` — so a local directory and an S3 bucket behave
alike, and handing the file to the bank channel over SFTP replaces that one function
without touching the kind. The service refuses an export that does not balance against its
bookings, so an unbalanced file is never handed over.

`POST /api/ledger/exports` (role `kasse`) queues one for a period: `{from, to}` -> `202`
with the job id (docs/27 describes the queueing seam).

## Not yet done

- Hand the file to the ledger/bank channel over SFTP (the artefact store is the swap point:
  the file is already written through it, so only the *delivery* is missing).
- The movements the bank sends *back* travel in the counterpart file
  (`cash-returns/v1`, `docs/30`), which this export's rows are reversed against.
- ~~The tie-back reconciliation~~ — done since: `ledger.Reconcile` runs on every export, the
  service **refuses** an export that does not balance its bookings, and the audit detail
  records `matched`/`missing`/`extra`/`balanced`, so a booking that did not export is visible
  rather than silent.
