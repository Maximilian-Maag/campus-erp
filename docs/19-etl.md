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

**Reference datasets with real data.** Two source tables hold real rows in the
productive source (most module tables are empty — `docs/legacy/kbs-cashbook.md`),
so these import measured data rather than an inference, and `lookup.StandardDatasets`
registers them:

- **Bank-code directory** — `kbs.blz` (3,732 rows, verified against the restored
  dump) into the `bank_leit_zahl` catalogue namespace: BLZ → name / city / BIC,
  directly useful for SEPA and IBAN work.
- **Health insurers** — `sva4.k_krankenkasse` (1,677 rows) into the `krankenkasse`
  namespace, reference data for the Personal module (Phase 3).

Both reuse `LegacyCatalogueSource` + `CatalogueSink`; the queries are the measured
legacy column shapes (fixed-width char columns trimmed and cast to text).

**Module datasets.** The cash-book module contributes four more, registered by
`kasse.StandardDatasets` into the same registry:

| dataset | legacy source | canonical |
|---|---|---|
| `kasse/vat` | `kbs.mwst_art` | `vat_type` |
| `kasse/tax-office` | `kbs.finanzamt` | `tax_office` |
| `kasse/foundation` | `kbs.stift` | `foundation` |
| `kasse/counter-book` | `kbs.schalterbuch` | `cash_book_day` + `cash_booking` |

Their sources and sinks are described below; the queries are the same measured
column shapes the integration tests use, addressed unqualified so the dataset
pool's `search_path` picks the schema.

**Registration is what makes a dataset reachable.** A source and a sink can be
written, tested and documented and still be dead: the registry is the only door,
and `cmd/api` and `cmd/worker` are the only places that build one. These four
datasets existed for several slices while neither runtime registered them, so
`GET /api/etl/datasets` listed two of six and running any of them answered
`unknown dataset`. Both runtimes now register `lookup.StandardDatasets` **and**
`kasse.StandardDatasets`, and `cmd/api/wiring_test.go` reads both composition
roots to keep it that way — a missing call cannot be observed from inside the
process that lacks it.

Registered is not the same as runnable *everywhere*: with no `ERP_LEGACY_DATABASE_URL`
the dataset pool is the canonical one, and these queries name legacy tables
(`mwst_art`, `finanzamt`, `stift`, `schalterbuch`) that the canonical schema does not
have — so a run fails at the source with a missing relation rather than reporting an
empty import. That is the honest failure: there is no extract to read. Against a real
extract the same run reads and reconciles.

## Running a dataset (API)

`etl.NewRegistry` binds a `*Runner` to a set of named datasets, and the API exposes
it so a re-run / reconciliation can be triggered on a real system (not only from
tests):

| method | path |
|---|---|
| `GET` | `/api/etl/datasets` — the registered dataset names |
| `GET` | `/api/etl/runs?dataset=…&limit=…` — the recent runs, newest first |
| `POST` | `/api/etl/runs` `{ "dataset": "..." }` — run one, returns the `etl.Report` |

All three are behind `erp-admin` (an import changes canonical data). An unknown dataset
name is a `404`. The runner's audit sink and ledger are the canonical store, so a
run continues the hash-chained audit stream and drift detection works as in a test.

`GET /api/etl/runs` is that stream **read back**: `audit.Store.Events(action, subject, limit)`
selects a dataset's runs (or, with no `dataset`, every dataset's), and the handler returns
`{ "runs": [{ "at", "actor", "summary" }] }`. The summary is the detail the run recorded
(`source=… loaded=… checksum=…`, plus `previous`/`drift` when a ledger was attached), so a
run's history needs no second store. `limit` must be a positive integer when given; the store
bounds it (default 20, max 200).

## Screen — *Importe* (`/importe`)

Lists the datasets the server registers (`GET /api/etl/datasets` — the screen carries no list of its
own, so it can only offer what exists), narrows the history to one dataset or shows every dataset's,
and starts a run of the chosen one.

- **The history is the audit stream read back.** A run's `etl.run` event *is* its record, so the
  screen shows what the run recorded — the source row count, the loaded count, the checksum, and
  whether the source drifted from the previous run — rather than a second bookkeeping of it
  (`docs/19` above, `GET /api/etl/runs`). The times are shown in **UTC and labelled as such**: the
  API records runs in UTC, so rendering them in a viewer's own zone would let two people read two
  different histories of one import.
- **Prevention over refusal.** The run button is disabled until a dataset is chosen, with the reason
  next to it, instead of sending an empty name and reporting the refusal back. The service still
  refuses an empty name (a caller that is not this screen must be told), but the screen does not
  route a person through that error.
- **The run is read back, not assumed.** After a successful run the screen reloads the history, so
  the newest entry is what the audit stream holds rather than what the response claimed.
- The routes behind this screen are the `erp-admin` ones (an import changes canonical data), which is
  the role this screen needs — unlike the domain screens, which sit behind their own module's role.

The datasets read from the **legacy** database; the sink writes the canonical one.
Set `ERP_LEGACY_DATABASE_URL` to point the source at the legacy DSN — when unset the
source is the canonical pool (dev / seed, where the synthetic source tables live).

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
- **Counter book** (`internal/modules/kasse/etl.go`,
  `docs/legacy/kbs-cashbook.md`): `LegacyCounterBookSource` reads
  `kbs.schalterbuch` and `CounterBookSink` imports each day through the cash-book
  service into `cash_book_day` + `cash_booking`. The legacy table is empty, so the
  mapping is an explicit inference: **amount sign → kind** (no booking-type code
  list exists in `kbs`), the purpose from `grund1`+`grund2`, and the day created
  closed with a computed closing balance. Idempotency comes from a `legacy_ref`
  natural key (migration `0016`).
- **Tax offices + foundations** (`internal/modules/kasse/etl.go`,
  `docs/legacy/kbs-tax-and-foundations.md`): `LegacyTaxOfficeSource` reads
  `kbs.finanzamt` and `LegacyFoundationSource` reads `kbs.stift`; the sinks load
  them into `tax_office` and `foundation`. These cover the two remaining shapes —
  an integer year (validated 1900..2999) and a carry-over flag. *(inferred)*: the
  legacy `kz_uebern char(1)` encoding is unverified (the sample dataset is empty), so
  `J`/`Y`/`1`/`T` mean yes and anything else (including empty) means no.

Each dataset's integration test builds a synthetic legacy table in an isolated
schema and asserts reconciliation, re-run idempotency, in-place update on a
changed source, fail-closed on a bad row, and a verifying audit chain.

## Deferred: the rest of the cash book

The counter book above is imported on an inferred mapping; the rest of the
cash-book group (`kbs.bu` the ledger-side bookings, `kbs.kass` the denomination
count, `kbs.geing`) stays out of scope: it is **empty** in the Muster (measured —
`docs/legacy/kbs-cashbook.md`), so no mapping can be validated, and `kbs.bu`
belongs to the ledger (Phase 4, `mbs`).

- ~~The run history was API-only~~ — done since: the *Importe* screen (above) lists the datasets, the
  runs and their recorded results, and starts a run. Reading it also fixed the contract it reads:
  `GET /api/etl/runs` documented `string[]` for a list of run objects and `EtlReport` an open object,
  because the `etl` package was missing from the generator's package list. The generator now refuses
  an unresolvable type instead of degrading the schema (`docs/15`).

## Not yet done

- The rest of the cash-book group as datasets — `kbs.bu` (the ledger-side bookings,
  Phase 4), `kbs.kass` (the denomination count: how a physical count maps onto
  opening/closing is a data decision, not a plumbing one) and `kbs.geing`. The
  counter book itself is a dataset (`kasse/counter-book`) — it was listed here as
  missing for a while after it existed, which is how the registration gap in
  *Datasets* went unnoticed. Each new one is a `Dataset` plus its name in `StandardDatasets`,
  not new plumbing, and the wiring test fails until both runtimes register it.
- The **payment orders / statements** tables as datasets: `docs/legacy/kbs-payment-orders.md`
  and `kbs-statement-import.md` measured them empty like the cash book.
- The **parallel run**: extract from a real legacy database, reconcile against it
  (row counts + checksums + domain invariants such as bookings-per-day), and
  rehearse the reversible cutover. Blocked on a production extract (ADR-0001).
- ~~A queryable history of runs beyond "the last one"~~ — done since:
  `GET /api/etl/runs` reads the audit stream back, per dataset or across datasets.
