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

## Not yet done

- The endpoint/run that hands the rendered file to the ledger side (issue #42,
  increment 3).
- The tie-back reconciliation: assert every exported posting maps to exactly one
  booking, and surface bookings that did not export.
