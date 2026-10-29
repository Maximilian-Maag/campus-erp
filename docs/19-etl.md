# 19 — ETL: re-runnable, reconcilable legacy → canonical

Phase 0 of the roadmap lists an ETL framework ("legacy → canonical, re-runnable +
reconcilable"); `kbs-pilot.md` S8 is its first consumer. This slice builds the
framework and one real pipeline — a legacy key table into the canonical catalogue
— against synthetic fixtures, since no production extract exists yet (ADR-0001).

## Model (`internal/platform/etl`)

A pipeline is a **`Dataset`**: a named `Source` (the legacy side) bound to a
`Sink` (the canonical side).

- `Record` — one extracted row: a natural `Key` (stable across runs) plus the
  canonical fields it maps to.
- `Source.Extract(ctx) ([]Record, error)` — reads the legacy rows. `QuerySource`
  is the schema-driven implementation: a dataset supplies the SQL and the
  row→`Record` mapper, so a new legacy table is a new query, not new plumbing.
- `Sink.Load(ctx, records, by) (int, error)` — writes the canonical rows. It must
  be idempotent (upsert by natural key) and must not partially apply a batch.
- `Runner.Run(ctx, ds, by) (Report, error)` — extracts, loads, reconciles, audits.

### Idempotency and reconciliation

- **Idempotent.** The sink upserts, so re-running over an unchanged source
  converges on the same state; the run's `Checksum` (a digest of the extracted
  set, independent of extract order) is the same on the second run. A changed
  source moves the checksum and updates values in place.
- **Fail closed.** A run whose loaded count does not match its source count
  returns `ErrReconcile` — a dropped row is an error, not a silent loss. A row
  that cannot be canonicalised fails the load with no partial write (the sink
  validates the whole batch before writing).
- **Audited.** Every run appends one `etl.run` event (actor, dataset, source /
  loaded counts, checksum) to the hash-chained stream (ADR-0006).

## Run ledger and drift detection

The run history is the hash-chained audit stream itself — no separate ledger table.
`audit.Store.LastEvent(action, subject)` reads the most recent matching event, and
`etl.AuditLedger` turns the last `etl.run` event for a dataset into that run's
checksum. Attached with `Runner.WithLedger`, each run then reports the previous
checksum and whether the source has **drifted** from it, so an import that changes
unexpectedly is visible rather than silent. The audit detail reads
`source=… loaded=… checksum=… previous=… drift=…`; `afterText` writes it and
`checksumOf` parses it, from one definition.

## Datasets

A dataset is a `Dataset` value; each is independent, and adding one is a new
source + sink, not new plumbing.

- **Lookup catalogue** (`internal/modules/lookup/etl.go`, `docs/11`):
  `LegacyCatalogueSource` reads one legacy key table (the productive source's
  3,532 coded value sets) and `CatalogueSink` loads it through the value service
  into the single canonical `lookup_value` catalogue.
- **VAT catalogue** (`internal/modules/kasse/etl.go`,
  `docs/legacy/kbs-tax-and-foundations.md`): `LegacyVATSource` reads `kbs.mwst_art`
  and `VATSink` loads it through the tax service into `vat_type`. The mapping is
  the documented S7 one — percent → integer basis points (19.00 → 1900), the
  1-char `U`/`V` flag → the enumerated kind, and no sign column. *(inferred)*: an
  unset flag defaults to output tax (the majority case) because the canonical
  model requires a kind; a truly input-tax row with an unset flag (legacy code 67)
  is misclassified until a production extract lets the mapping be reviewed.
- **Tax offices + foundations** (`internal/modules/kasse/etl.go`,
  `docs/legacy/kbs-tax-and-foundations.md`): `LegacyTaxOfficeSource` reads
  `kbs.finanzamt` and `LegacyFoundationSource` reads `kbs.stift`; the sinks load
  them into `tax_office` and `foundation`. These cover the two remaining shapes —
  an integer year (validated 1900..2999) and a carry-over flag. *(inferred)*: the
  legacy `kz_uebern char(1)` encoding is unverified (the Muster dump is empty), so
  `J`/`Y`/`1`/`T` mean yes and anything else (including empty) means no.

Each dataset's integration test builds a synthetic legacy table in an isolated
schema and asserts reconciliation, re-run idempotency, in-place update on a
changed source, fail-closed on a bad row, and a verifying audit chain.

## Deferred: the cash book

The cash-book tables (`kbs.schalterbuch`, `kbs.bu`, `kbs.kass`, `kbs.geing`) are
structurally complete but **empty** in the Muster dump (measured — see
`docs/legacy/kbs-cashbook.md`), so their mapping cannot be validated and the
dataset is deliberately not built yet. Two design points are settled for when a
production extract arrives: `cash_booking` needs a natural key (a migration adding
a `legacy_ref`), and the import must go through the day-lifecycle service so every
write stays on the audit chain.

## Not yet done

- The remaining `kbs` tables (cash book, payment orders, statements) as datasets —
  S8 proper. Each is a new `Dataset`, not new plumbing.
- The **parallel run**: extract from a real legacy database, reconcile against it
  (row counts + checksums + domain invariants such as bookings-per-day), and
  rehearse the reversible cutover. Blocked on a production extract (ADR-0001).
- A queryable history of runs beyond "the last one" (the audit stream holds every
  run, but there is no read API that lists them by dataset yet).
