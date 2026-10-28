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

## First pipeline: the lookup catalogue

`internal/modules/lookup/etl.go` wires the framework to the catalogue
(`docs/11`): `LegacyCatalogueSource` reads one legacy key table (the productive
source's 3,532 coded value sets, each a table with code / texts / order / active)
and `CatalogueSink` loads it through the value service into the single canonical
`lookup_value` catalogue.

The integration test creates a synthetic legacy table in an isolated schema, runs
the pipeline and asserts reconciliation, re-run idempotency, in-place update on a
changed source, fail-closed on a bad row, and a verifying audit chain.

## Not yet done

- The remaining `kbs` tables (cash book, payment orders, statements) as datasets —
  S8 proper. Each is a new `Dataset`, not new plumbing.
- The **parallel run**: extract from a real legacy database, reconcile against it
  (row counts + checksums + domain invariants such as bookings-per-day), and
  rehearse the reversible cutover. Blocked on a production extract (ADR-0001).
- A queryable history of runs beyond "the last one" (the audit stream holds every
  run, but there is no read API that lists them by dataset yet).
