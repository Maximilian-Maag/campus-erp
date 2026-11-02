# Changelog

All notable changes to this repository. Format: Keep a Changelog.

## [0.57.0] - 2026-11-02

Worker: the report-render job kind (closes the code work on #38).

### Added
- `jobs.KindReportRender` with `ReportRenderArgs{ReportID, From, To, Actor}` and
  `ReportRenderWorker`: builds a report by id over a period through `ReportRenderFunc`,
  refuses a missing id, a malformed period and a render that stored nothing, and returns
  failures so River retries. `EnqueueReportRender` is the API-side seam.
- Registered in `StandardWorkers`; the worker role knows `kasse-daily-close`, collects the
  period's bookings, renders the report and stores the PDF through the document service and
  the artefact store.
- Tests: the kind, the happy path, every refusal, the logger. Docs: `docs/17` §7 and
  `docs/25`.

## [0.56.0] - 2026-11-02

Storage: the S3 path is verified.

### Changed
- `storage.TestS3RoundTrip` now serves an in-process S3-compatible server (gofakes3) when
  `ERP_S3_TEST_ENDPOINT` is unset, so the S3 store is exercised on every build instead of
  being written-but-unproven: bucket creation (idempotent), Save/Get round trips with nested
  keys, a missing key failing, and an escaping key refused — all through the minio-go client.
  `ERP_S3_TEST_ENDPOINT` still points the same test at a real endpoint.
- MinIO's own image is refused by this environment's registry, so verification runs against
  an S3-compatible server rather than MinIO itself. ADR-0011 and `docs/26` updated.

## [0.55.0] - 2026-11-02

Storage: one keyed artefact store (ADR-0011).

### Added
- `internal/platform/storage`: `Store` (`Save`/`Get`), a directory implementation and an
  S3-compatible one (minio-go), plus `FromEnv` (`ERP_S3_ENDPOINT` → S3, else a directory)
  and `CheckKey` (empty/absolute/escaping keys refused). Mutation target.
- The three artefact writers now share it: dunning notices (`cmd/api`), and the ledger and
  SEPA exports (`cmd/worker`), with keys `dunning/<id>.pdf`, `ledger/…csv`, `sepa/…xml`.
  `documents.Store` is the one-method subset, so no adapter is needed.
- **ADR-0011** and `docs/26-storage.md`, which states plainly that the S3 path is
  written but not yet verified end to end (the MinIO image could not be pulled here).

## [0.54.0] - 2026-11-02

Reports: the model and the first report (part of #38).

### Added
- `internal/core/report`: a report is a `Definition` (id, title, ordered columns with key,
  heading and alignment) plus rows of strings. `Validate` refuses an incomplete definition,
  duplicate keys and rows that do not match the columns; `MarshalCSV` and `MarshalHTML`
  render it, with everything escaped by `html/template`. Mutation target.
- `kasse.DailyCloseReport` + `DailyCloseDefinition`: the cash book's daily-close report —
  one row per booking and a closing row with income, expense and the net.
- **ADR-0010** (a report is a definition plus rows, and why not a template per report) and
  `docs/25-reports.md`.

## [0.53.0] - 2026-11-02

Worker: the sepa-export job kind (part of #38).

### Added
- `jobs.KindSepaExport` with `SepaExportArgs{RunID, Actor}` and `SepaExportWorker`:
  delivers a payment run's already-rendered pain.001 file to the bank channel through
  `SepaExportFunc`. Refuses a missing run, an empty file name or content, and a name
  that is nested or escapes the outbox; failures are returned so River retries.
- Registered in `StandardWorkers`; the worker role fetches the run file from the
  cash-book payment-run repository and writes it to `ERP_SEPA_OUTBOX_DIR`.
- Tests: the kind, the happy path, every refusal, the logger.

## [0.52.0] - 2026-11-02

Dunning notices get their document (increment 2 of #39).

### Added
- `documents.Store` and `documents.DirStore`: keep rendered documents under a key,
  creating parents; a key that is empty, absolute or escapes the root is refused.
- `DunningNotice.DocumentKey` and `kasse.DunningRepository.SetNoticeDocument`
  (migration 0019): the notice row carries the reference to its stored artefact.
- `kasse.DunningDocuments` + `DunningService.WithDocuments`: issuing a notice renders
  and stores its PDF and records the key. A render or store failure fails the issue, so
  a notice never claims a document it does not have; the audit detail names the key.
- The API role builds the issuer from `ERP_DOCUMENTS_URL` and `ERP_DOCUMENTS_DIR`
  (`ERP_ORGANISATION` for the issuer line); with either unset, notices are issued
  without a document instead of failing.
- Tests: the store (save, nesting, every refusal), the hook (key recorded, audit names
  it, no documents -> no key, render failure, record failure).

## [0.51.0] - 2026-11-02

Ledger export: the worker job kind and the hand-over seam (increment 3 of #42).

### Added
- `jobs.KindLedgerExport` with `LedgerExportArgs{From, To, Actor}` and
  `LedgerExportWorker`: renders a period's export and hands it over via
  `LedgerExportFunc`; refuses a missing actor or a malformed period and returns a
  failure so River retries it. `EnqueueLedgerExport` is the API-side entry point.
- `LedgerExportService.Export` now enforces the tie-back invariant: an export that
  does not balance against its bookings is refused (`ErrLedgerUnbalanced`) and never
  handed over.
- The worker role builds the exporter from the cash-book ledger service and writes the
  file to `ERP_LEDGER_OUTBOX_DIR` (the seam object storage or SFTP replaces).
- Registered in `StandardWorkers`; tests for the kind, the happy path, every refusal,
  the logger and the invariant.

## [0.50.0] - 2026-11-02

Ledger export: tie-back reconciliation (increment 3 of #42, first half).

### Added
- `ledger.Reconcile`: compares the booking references of a period with the postings
  rendered from them, counts each side (so a duplicate cannot hide behind a matching
  reference) and reports the surplus as `Missing`/`Extra` with `Balanced()`.
- `LedgerExportService.Export` now reconciles the run and records it on the audit
  stream (`matched`, `missing`, `extra`, `balanced`), so an export that does not
  account for its bookings is visible rather than silent.

## [0.49.0] - 2026-11-02

Worker: the statement-import job kind (part of #38).

### Added
- `jobs.KindStatementImport` with `StatementImportArgs` and `StatementImportWorker`:
  imports one bank-statement file (content travels with the job, so a retry re-imports
  the same bytes), refuses a missing account or empty content, and returns failures so
  River retries them. `EnqueueStatementImport` is the API-side entry point.
- `StatementImportFunc` keeps the jobs package free of a dependency on the module that
  owns the import, exactly as `DatasetRunner` does for ETL.
- Registered in `StandardWorkers`; the worker role builds the importer from the cash-book
  statement service. Tests for the args kind, the happy path, every refusal and the
  logger.

## [0.48.0] - 2026-11-02

Documents: HTML notice + PDF service (part of #39).

### Added
- `internal/platform/documents`: `DunningNotice.HTML` (deterministic, escaped HTML from
  a dunning case; golden-tested) and `Renderer` / `NewHTTPRenderer`, which posts the
  HTML to the document service and returns the PDF. Mutation target.
- One new deploy component: the document service (stateless HTML→PDF), gated in the
  chart (`pdf.enabled`, off by default) with values in `values.yaml`.
- **ADR-0009** (render documents with a service, and why not an in-process PDF
  library), `docs/24-documents.md`, and a short target-container overview with the
  document path in `docs/c4/02-container.md`.

## [0.47.0] - 2026-11-02

Ledger posting export: collection + service (increment 2 of #42).

### Added
- `kasse.LedgerRepo.ListBookingsForExport`: every booking made on a day inside the
  period, in (business date, booking id) order. The reference is the import reference
  when the booking has one and a derived `BK-######` otherwise - it is the export's
  idempotency key.
- `kasse.LedgerExportService.Export`: maps each booking to a posting (income credits
  the counterpart account, expense debits it), renders the `cash-postings/v1` export,
  and records the run on the audit stream. It refuses a blank actor, incomplete
  accounts, an inverted period and an empty period rather than emitting an empty file.
- `kasse.LedgerAccounts` (+ `DefaultLedgerAccounts`) for the cash/income/expense
  account mapping, validated before use.
- Tests: the posting mapping, ordering and determinism, the full error table
  (actor, accounts, period, empty period, repository, unknown kind, audit), and an
  integration test that seeds a day with two bookings and exports them twice -
  byte-identical - plus the empty-period refusal.

## [0.46.0] - 2026-11-02

Ledger posting export, format + renderer (increment 1 of #42).

### Added
- `internal/core/ledger`: the `cash-postings/v1` export that carries confirmed cash
  bookings to the ledger side. `Marshal` is versioned, validated, sorted by booking
  date then reference and field-trimmed, so the same booking set always renders
  byte-identically; references are the idempotency key. Mutation target (100 %).
- `docs/23-ledger-postings.md`: the format and how it is consumed.

## [0.45.0] - 2026-11-02

Make the repository vendor-neutral.

The project is an independent Go rebuild; the legacy implementation and its data are
only a reference. Nothing in the repository may point at a specific commercial
product, so every vendor identifier was removed (~600 occurrences across ~60 files):

- Product/company names (the campus-management and legacy-ERP brands, the vendor's
  name and domain) replaced with neutral wording: "the legacy system", "the reference
  implementation", "the vendor".
- Vendor module product codes replaced by their domain ("the cash-book module",
  "the HR module", ...), and vendor package/file paths replaced by role descriptions
  ("the reference parser", "the reference application", "the SOAP service catalogue").
- The interface registry's vendor entries are now keyed generically (`erp-soap`,
  `card-terminal`) with neutral names, and the contract/registry golden were
  regenerated.
- The module catalogue and the source inventory no longer cite vendor marketing or
  file inventories; they describe the domains to be rebuilt.

Verified: no vendor token remains anywhere in the tracked tree; policy_check green;
`make test` green (4 suites, fail=0); contract regenerated.

## [0.44.0] - 2026-11-02

Prepare the repository for public release.

### Removed
- `schema/` - a `pg_dump --schema-only` of the proprietary third-party database plus
  its derived TSVs (~7.6 MB). It is generated from that dump by `scripts/analyze.sh`,
  so it is now git-ignored and reproducible locally instead of committed. The docs
  that quoted measurements from it still do, and now say how to regenerate it.

### Added
- `LICENSE` (Apache-2.0) and `CONTRIBUTING.md`.
- README: a "what is not in this repository" section (the vendor dump, the generated
  artefacts, the vendor XSDs/Java/JARs), a trademark note (the reference implementation/the legacy university ERP are
  the vendor products; this is an independent reimplementation), and an up-to-date status
  paragraph (the old one still said CAMT.053 was unimplemented).

### Fixed
- `tools/gen_erm.py` / `tools/test_erm.py`: the legacy ER models are generated from
  `schema/`, so the generator and its self-test now degrade gracefully when that
  directory is absent (target model only, legacy checks skipped) instead of failing.

### Changed
- `.github/workflows/rerun-flaky.yml` renamed to `rerun-on-runner-shutdown.yml` to
  match what the changelog and the workflow's own name say.

## [0.43.0] - 2026-11-02

CI: retry runs killed by a runner preemption, and only those (#41).

### Added
- `.github/workflows/rerun-on-runner-shutdown.yml` — on `quality` completion, if a run
  failed and **every** failed job's log carries the runner-shutdown marker
  (`received a shutdown signal`, exit 143), it re-runs the failed jobs. It declines
  when any failure looks real, when there are no failed jobs, and when the run has
  already been re-run once (`run_attempt > 1`), so it cannot loop. Also runnable via
  `workflow_dispatch` with a `run_id` and a `dry_run` flag for inspection.

## [0.42.0] - 2026-11-01

XSD validation wired up, and the bugs it found (#40).

Wiring `golden.ValidateXSD` into the SEPA and CAMT tests (schema path from
`ERP_XSD_PAIN001/Pain008/CAMT052/053/054`, skipped when unset) exposed real defects:

### Fixed
- **CAMT parsers read the counterparty from a non-ISO path.** `Dbtr/Nm` and
  `Dbtr/Acct/Id/IBAN` do not exist in ISO 20022 — a party is a `Party40Choice`
  (`Dbtr/Pty/Nm`) and the account is a sibling (`DbtrAcct/Id/IBAN`). On a real bank
  file the counterparty name and IBAN were therefore empty. Fixed in camt.052/053/054.
- **pain.008 emitted an invalid creditor scheme name** (`SchmeNm/Cd`); the GBIC
  profile requires `Prtry`.
- **pain.008 made `DbtrAgt` optional**, but it is 1..1: a transaction without a
  debtor BIC now fails with `ErrDebtorAgentRequired` instead of producing a file the
  bank rejects.

### Changed
- The CAMT test fixtures are now schema-valid (they were hand-written and had never
  been validated): `Sts`, `BkTxCd`, and the ISO party/account nesting.

## [0.41.0] - 2026-11-01

Platform: the worker runtime (River) — the last Phase-0 gap (#44).

### Added
- **`internal/platform/jobs`** — the async runtime on River/Postgres (ADR-0007):
  `Migrate` (idempotent River schema), `New` (client with the standard workers),
  `StandardWorkers`, and the `etl` job kind (`EtlArgs{Dataset, Actor}`) which runs a
  registered dataset and puts the run on the audit stream. `EnqueueEtl` is the
  enqueue seam for the API role.
- `cmd/worker` is wired: opens the pool, migrates, builds the ETL registry (legacy
  source pool when `ERP_LEGACY_DATABASE_URL` is set), starts the client, blocks until
  SIGINT/SIGTERM and drains with a bounded stop.
- Tests: the `etl` kind's args/error paths, and an integration test that migrates
  River's schema, starts a client, enqueues a job and waits for it to complete.

### Changed
- The module moves to **Go 1.26** (River v0.51 requires it): `go.mod` and the CI
  workflows' `setup-go`. The local docker fallback image follows (`golang:1.26-alpine`).
- `internal/platform/jobs` is a mutation target (100 % efficacy).

## [0.40.0] - 2026-11-01

Bank statement import: CAMT.052 (intraday account report) (#34).

### Added
- **`internal/core/camt052`** — parses an ISO 20022 camt.052 bank-to-customer
  account report into the shared `bankstmt` shape; `Bal` `OPBD`/`CLBD` are mapped
  when present and left zero otherwise. Mutation target added.
- `StatementService.Import` routes the XML container: `BkToCstmrAcctRpt` →
  CAMT.052, `BkToCstmrDbtCdtNtfctn` → CAMT.054, otherwise CAMT.053.
- Tests: a golden report (balances + credits + debits), the no-balance case, error
  cases, amount/date parsing, and an integration test reconciling a CAMT.052 line
  against a paid order.
- `bank-camt052` added to the interface registry (IR-2); the statement-import doc and
  the C4 external-systems diagram note the format.

## [0.39.0] - 2026-11-01

Tooling: split the quality gate into a fast suite and the mutation gates (#32).

### Changed
- `tools/run_tests.sh` takes a `STEPS` selector: `fast` (Go vet/build/unit+integration,
  Angular build + unit, the ER self-test, the API-contract drift guard) and
  `mutation` (gremlins per package + Stryker); default is both.
- **`make test`** now runs the fast suite (seconds when warm — it previously ran
  everything and took ~13 min, which is over an interactive command budget);
  **`make mutation`** runs the mutation gates alone; **`make test-all`** runs both.
  `make quality` runs policy + contract + the full suite. CI is unaffected (it
  already splits these into separate jobs).
- README / AGENTS / `docs/build-and-deploy.md` updated for the split.

## [0.38.0] - 2026-10-28

ETL: run datasets from the API (#26).

### Added
- **`etl.Registry`** — binds a `Runner` to named datasets (`Names`, `Run`,
  `ErrUnknownDataset`), so a run no longer needs hand-wiring.
- **API** (contract-first): `GET /api/etl/datasets` and `POST /api/etl/runs`
  `{"dataset":"..."}` (returns the `etl.Report`), behind `erp-admin`.
  `contracts/openapi.yaml` + generated client regenerated (58 operations,
  65 schemas); drift guard green.
- `ERP_LEGACY_DATABASE_URL` — the ETL source pool (legacy DSN); when unset the
  canonical pool is used (dev / seed). `cmd/api` opens the second pool and builds
  the registry from `lookup.StandardDatasets`.
- Tests: registry (names sorted, blank/duplicate handling, delegation, unknown
  name), the HTTP handlers, and the status mapping.

## [0.37.0] - 2026-10-28

Bank statement import: CAMT.054 (#23).

### Added
- **`internal/core/camt054`** — parses an ISO 20022 camt.054 debit/credit
  notification into the shared `bankstmt` shape. A notification carries no
  balance, so the parsed statements hold lines only. Mutation target added.
- `StatementService.Import` detects the XML container: `BkToCstmrDbtCdtNtfctn`
  → CAMT.054, otherwise CAMT.053; both reconcile identically (matched / returned /
  unmatched, idempotent on the file hash).
- Tests: a golden notification (credits + debits, counterparty, purpose, dates),
  the no-balance invariant, error cases, amount/date parsing, and an integration
  test that imports a CAMT.054 and reconciles a line against a paid order.
- `bank-camt054` added to the interface registry (IR-2); `docs/legacy/kbs-statement-import.md` notes the format.

## [0.36.0] - 2026-10-28

SEPA direct-debit collection run (#22).

### Added
- **`direct_debit_run`** + **`direct_debit_run_item`** (migration `0018`) — one
  produced pain.008 file per collection date/sequence type, with the stored file,
  its hash, and one row per collected mandate (`UNIQUE (run_id, mandate_reference)`).
- **`kasse.CollectionRun`** domain + repo + audited service `CreateCollectionRun`:
  resolves each item's mandate, builds a `sepa.DirectDebit`, stores the file, and
  is idempotent on the reference. Fails closed on an unknown/inactive mandate, a
  sequence-type mismatch, a blank reference or a duplicate mandate in one run.
- **API** (contract-first): `POST /api/direct-debit/runs` and
  `GET /api/direct-debit/runs/{reference}/file`, behind `erp-kasse`.
  `contracts/openapi.yaml` and the generated client regenerated (56 operations,
  63 schemas); the drift guard passes.
- Tests: run-level validation (every branch), an integration test (produced file
  bytes + hash, idempotent re-run, unknown mandate, sequence mismatch, the DB-level
  per-run uniqueness, audit), the HTTP handlers and the status mapping.
- `docs/22-collection-runs.md`; the interface registry's `bank-sepa-sdd` reference
  now points at the run.

## [0.35.0] - 2026-10-28

SEPA direct-debit mandates.

### Added
- **`direct_debit_mandate`** (migration `0017`) — the mandate reference, debtor,
  signature date and sequence type, with a constrained sequence type and no hard
  deletes.
- **`kasse.Mandate`** + `NewMandate` validation (reference, ISO 7064 IBAN, ISO 9362
  BIC via `core/sepa`, signature date, sequence type) and the mandate service
  (`mandate.create` audited).
- **API** (contract-first): `POST /api/direct-debit/mandates` and
  `GET /api/direct-debit/mandates/{reference}`, behind `erp-kasse` like dunning.
  `contracts/openapi.yaml` and the generated Angular client regenerated (54
  operations, 60 schemas).
- Tests: domain validation, an integration test (persist + get + duplicate +
  missing + audit), the HTTP handlers, and the status mapping.
- `docs/21-mandates.md`.

## [0.34.0] - 2026-10-28

Interface: SEPA direct debit (pain.008.001.08).

### Added
- **`sepa.DirectDebit`** (`internal/core/sepa/pain008.go`) — generates an ISO 20022
  pain.008.001.08 direct-debit initiation, the collection counterpart of the
  credit transfer: stdlib-only, deterministic, pinned by a golden file. The SEPA
  creditor identifier goes in `CdtrSchmeId`, each transaction carries its mandate
  (`MndtId` + `DtOfSgntr`), and sequence type / local instrument are constrained to
  the SEPA sets. Full validation with sentinel errors (`docs/20`).
- Tests: a golden file, determinism, structure (incl. optional agent blocks and
  empty-currency default), the length-limit boundaries and every validation branch.

### Changed
- The interface registry marks `bank-sepa-sdd` **implemented** (was planned);
  `testdata/interfaces.txt` regenerated.

## [0.33.0] - 2026-10-28

Deployment: ADR-0007 infrastructure in the chart, gated and off by default.

### Added
- **CloudNativePG `Cluster`** (`postgres-cluster.yaml`) and a **PgBouncer `Pooler`**
  (`pgbouncer-pooler.yaml`), one cluster with one database per tenant (ADR-0002).
- **Scheduler `CronJob`** (`scheduler.yaml`) for the nightly batch.
- **Redis `Deployment` + `Service`** (`redis.yaml`) for sessions / rate-limit /
  cache.
- `values.yaml` gains a gated ADR-0007 section (postgres, scheduler, redis,
  objectStorage); each piece can instead point at a managed/on-prem equivalent.
- `docker-compose.yml` gains MinIO and Redis, so local / single-host installs match
  the production shape.

### Changed
- `deploy/README.md` documents the ADR-0007 map (chart-provided vs external); ADR-0007
  records the revision.

## [0.32.0] - 2026-10-28

ETL: the two reference datasets that hold real data (bank codes, insurers).

### Added
- **`lookup.BankDirectoryDataset`** — imports the German bank-code directory
  (`kbs.blz`, **3,732 rows verified against the restored dump**) into the
  `bank_leit_zahl` catalogue namespace: BLZ → name / city / BIC. Useful for SEPA
  and IBAN work.
- **`lookup.InsurersDataset`** — imports the statutory health insurers
  (`sva4.k_krankenkasse`, **1,677 rows**) into the `krankenkasse` namespace.
- **`lookup.StandardDatasets`** — the registry of reference datasets sourced from
  the productive legacy data.
- An integration test driving both datasets against synthetic tables of the same
  measured shape, including re-run idempotency.

Both reuse the existing catalogue pipeline (`LegacyCatalogueSource` +
`CatalogueSink`), so a reference list is rows, not a new table.

## [0.31.0] - 2026-10-28

ETL: the counter book (kbs.schalterbuch) as a cash-book dataset.

### Added
- **`kasse.LegacyCounterBookSource` / `CounterBookSink`** — imports the legacy
  counter book into `cash_book_day` + `cash_booking`, grouped by business day.
  The legacy table is empty, so the mapping is an explicit inference: **amount sign
  → kind** (no booking-type code list exists in `kbs`), purpose from
  `grund1`+`grund2`, and the day created closed with a computed closing balance
  (`docs/legacy/kbs-cashbook.md`).
- **Migration `0016`** — a `legacy_ref` column on `cash_booking` with a
  partial-unique index, giving imported bookings a natural key for idempotent
  upserts without weakening a day's own integrity.
- **`Service.ImportCounterBook`** and the repo's `UpsertImportedDay` /
  `UpsertImportedBooking` / `SetDayClosing` — a validate-all-then-write import that
  goes through the day service (one `cashbook.import` audit event per day), so
  every write stays on the audit chain.
- Unit tests for the sign→kind / amount mapping and the import validation; an
  integration test for reconciliation, per-day closing balance, idempotency and
  fail-closed behaviour.

## [0.30.0] - 2026-10-28

Cash-book legacy findings (measured), and why the dataset is deferred.

### Added
- **`docs/legacy/kbs-cashbook.md`** — the FR-K1 group measured from the restored
  sample dataset: `schalterbuch` / `bu` / `kass` / `geing` / `dr` are all **empty**
  (0 rows), so the mapping is unvalidatable before a production extract (the same
  conclusion as payment orders); the three god tables (152 / 115 / 96 columns), the
  denomination-count table, and the absence of any booking-type code list.

### Changed
- **Correction:** `docs/plan/kbs-pilot.md` grouped `sb`/`sbprot` under the cash
  book; they are the **clerk (Sachbearbeiter) master + change log**, with a
  per-area `tabr_*` rights model. Recorded, with a structural security finding on
  the unsalted-looking `sb.pass_wort`.
- `docs/19` records the cash-book dataset as deferred, with the two settled design
  points (a `legacy_ref` natural key and routing the import through the day
  service).

## [0.29.0] - 2026-10-28

ETL: tax offices (kbs.finanzamt) and foundations (kbs.stift), completing S7.

### Added
- **Tax offices + foundations datasets** — `LegacyTaxOfficeSource` /
  `TaxOfficeSink` and `LegacyFoundationSource` / `FoundationSink` move
  `kbs.finanzamt` and `kbs.stift` into canonical `tax_office` and `foundation`.
  They add the two remaining dataset shapes: an integer year and a carry-over
  flag (docs/19); the unset-flag encoding is marked inferred.
- **`TaxRepository.UpsertTaxOffice` / `UpsertFoundation`** and
  **`TaxService.ImportTaxOffices` / `ImportFoundations`** — idempotent, once-audited
  batch imports (`tax_office.import`, `foundation.import`) that validate the whole
  batch before writing.
- Tests: the flag mapping and an integration test covering both pipelines
  (reconciliation, idempotency, and fail-closed on a blank description).

## [0.28.0] - 2026-10-28

ETL: the VAT catalogue (kbs.mwst_art) as a second dataset.

### Added
- **`kasse.LegacyVATSource` / `kasse.VATSink`** — move the legacy VAT catalogue
  into canonical `vat_type`: percent → integer basis points, the 1-char `U`/`V`
  flag → the enumerated kind, no sign column (docs/19). An unset flag defaults to
  output tax, marked as inferred.
- **`TaxRepository.UpsertVATType`** and **`TaxService.ImportVATTypes`** — an
  idempotent, once-audited batch import (`vat_type.import`) that validates the
  whole batch before writing, so an ETL re-run converges and a bad row fails
  without a partial load.
- Unit tests for the percent/basis-point and flag/kind mapping; an integration
  test covering reconciliation, re-run idempotency, drift, in-place update and
  fail-closed behaviour on synthetic legacy rows.

## [0.27.0] - 2026-10-28

Deployment: wire the platform config into the chart and compose.

### Fixed
- The api's Kubernetes **readiness** probe now targets `/readyz` (which pings the
  database) instead of `/healthz`; liveness stays `/healthz`, so a database blip
  no longer marks a serving process not-ready nor restarts it (NFR-6).
- The chart and `docker-compose.yml` now pass the platform's configuration to the
  api: `ERP_TENANT_DSN` (multi-tenant DSN template, from the db Secret) and
  `ERP_OIDC_ISSUER`/`ERP_OIDC_AUDIENCE`/`ERP_OIDC_JWKS_URL` — without which a
  deployed api could not authenticate or serve more than one tenant (ADR-0002/0003).
- The compose api gains a `/readyz` healthcheck.

## [0.26.0] - 2026-10-28

ETL: the run history is the audit stream, so a run flags source drift.

### Added
- **`audit.Store.LastEvent(action, subject)`** — read the most recent matching
  event, so a caller can look back at what it recorded previously.
- **`etl.AuditLedger` / `Runner.WithLedger`** — a dataset's previous checksum is
  derived from its last `etl.run` audit event (no separate ledger table). A run
  then reports `Previous` and `Drift` when the source has changed since last
  import; `afterText`/`checksumOf` keep the write and parse of the detail in step.

### Changed
- `docs/19` documents the ledger; the integration test asserts no-drift on an
  unchanged re-run and drift after a source change.

## [0.25.0] - 2026-10-28

ETL: the re-runnable, reconcilable legacy → canonical pipeline (roadmap S8).

### Added
- **`internal/platform/etl`** — a `Dataset` (named `Source` → `Sink`), a `Runner`
  whose runs are idempotent, reconcile (`ErrReconcile` on a dropped row), fail
  closed on a bad row (no partial load) and append one `etl.run` audit event. A
  schema-driven `QuerySource` turns a legacy table into records via a SQL query +
  mapper (`docs/19`).
- **First pipeline** — `internal/modules/lookup/etl.go` moves a legacy key table
  into the single canonical catalogue (`LegacyCatalogueSource` + `CatalogueSink`),
  with an integration test covering reconciliation, re-run idempotency, in-place
  update and fail-closed behaviour against synthetic fixtures.

### Changed
- `backend/mutation.json` grades `internal/platform/etl`; the stale
  `internal/modules/` exemption (module placeholders) is removed — the modules
  are graded targets.
- `docs/plan/kbs-pilot.md` records S8 as in progress; the parallel run remains
  blocked on a production extract (ADR-0001).

## [0.24.0] - 2026-10-28

Authorisation: per-route role gates on top of authentication.

### Added
- **Per-route authorisation** — `httpapi.authorize` / `authorizeRead` wrap every
  `/api/*` route with the role that owns it (`erp-kasse`, `erp-finanzen`,
  `erp-klr`, `erp-personal`; reads also admit `erp-reader`; `erp-admin` may call
  anything). The gate fails closed if a route is reached without a principal, and
  is a pass-through only when no verifier is configured. The policy lives next to
  the route, so it cannot drift from the table (`docs/17`).
- `internal/httpapi/authz_test.go` — the authorisation matrix (module role,
  reader, admin, wrong module, no role, open route, missing and bad tokens).

### Changed
- `docs/17` records the role model; role-based authorisation is no longer an
  open item.

## [0.23.0] - 2026-10-26

Platform spine: authentication, tenancy, readiness and the interface registry.

### Added
- **`internal/platform/auth`** — stdlib OIDC: RS256 JWTs verified against a JWKS
  (`JWKVerifier`, `RemoteKeySet`, `StaticKeySet`) with a `Principal` and coarse
  roles. The HTTP layer requires a bearer token on `/api/*` when configured and
  records the subject as the audit actor; `docs/17`.
- **`internal/platform/tenant`** — host-based tenant resolution, a DSN-template
  pool registry (lazy open + migrate, database-per-tenant) and `httpapi.Tenancy`,
  a multi-tenant request front. Wired in `cmd/api` via `ERP_TENANT_DSN`; proven by
  an integration test against two real databases (ADR-0002).
- **`/readyz`** — readiness that pings the database and returns 503 when it is
  down (NFR-6); `/healthz` stays pure liveness.
- **`internal/platform/interfaces`** — a validated registry of the external
  interfaces (bank, e-invoicing, OIDC, LBV, ELSTER, statistics, DMS, the reference implementation
  SOAP, KfW, BSOS terminal), seeded from requirements §5 and `docs/14`, pinned
  by a golden file.
- **`internal/core/golden`** — a golden-file + XSD test harness used by the SEPA
  and XRechnung tests.

### Added (test infrastructure)
- **`internal/platform/dbtest`** — an integration-test harness that gives each
  test **process** its own schema (`test_<pid>`, dropped on exit), so concurrent
  test binaries never share tables. All packages' `testPool` delegate to it.

### Fixed
- **Database flakiness.** `TestPaymentRunRejectsInvalidCreditorBIC` failed
  intermittently under `go test ./...`: every test binary shared one database,
  `payment_recipient` is keyed by IBAN, `CreateRecipient` upserts the BIC, and the
  fixtures reused a fixed IBAN — so a concurrent binary could overwrite the BIC
  the assertion depended on. Fixed by per-process schema isolation plus unique
  IBAN fixtures (`docs/18-test-isolation.md`).

### Changed
- `contracts/openapi.yaml` gains `401`/`403` responses on every operation.
- `mutation.json` adds `internal/platform/auth` and `internal/platform/tenant`;
  the test harness (`internal/core/golden`) is exempt.
- ADR-0002 and ADR-0003 record that tenancy and auth are now implemented.

## [0.22.0] - 2026-10-26

Contract-first made real: an executable API contract, a generated Angular
client and its first consumer, plus the rest of the reference implementation bundle analysed.

### Added
- **`docs/14-bundle-rest-and-interfaces.md`** — the un-inventoried part of the
  4.2 GB bundle: `the legacy front-end` (the legacy ERP's own front-end, JasperReports + Saiku
  + the `dbt` schema tool), the **SOAP surface** (79 services / 638
  operations, incl. `PaymentExportService`, `CardTerminalService`,
  `RightsAndRolesService`) and the shipped **connectors** (`the card-terminal connector`
  chip-card bridge, `the student-loan client` KfW SOAP client).
- **`tools/gen_openapi.py`** — generates `contracts/openapi.yaml` from the Go
  route table + structs (49 paths, 51 operations, 58 schemas; transitive refs
  resolved, no permissive schemas).
- **`tools/api_contract.py`** — stdlib-only drift guard (`check`), Angular
  client generator (`gen`) and self-test (`selftest`). Fails closed: an
  undocumented Go route or a dangling `$ref` breaks the build.
- **Generated Angular client** `frontend/src/app/core/api/generated/`
  (`models.ts`, `api.service.ts`) from the contract (ADR-0003).
- **First consumer** `frontend/src/app/kasse/cashbook-day.component.ts` — *Kasse:
  open a cash-book day* — with a Karma spec; route `/kasse/tag`.
- **`internal/httpapi/middleware.go`** — correlation id, panic recovery and a
  structured access log (`log/slog`) around the router (NFR-5), with tests.
- **`httpapi.Deps`** — `New` takes a dependency struct instead of thirteen
  positional interfaces; tests supply only what they exercise.
- **Unified error envelope** — every handler now writes the JSON `Problem`
  shape from the contract (`writeProblem`), not plain-text `http.Error`.
- **Nullable fields** — pointer Go fields emit `x-nullable`; the generated
  TypeScript is `number | null`, so the client stops lying about absent values.
- **`docs/15-api-contract.md`** and **`docs/16-http-transport.md`** — how the
  contract pipeline works, and the transport layer that serves it.

### Changed
- `contracts/openapi.yaml` grew from 2 documented routes to all 51; the phantom
  `POST /audit` is gone.
- `tools/run_tests.sh`, `Makefile` (`contracts-check`, `contracts-gen`,
  `contracts-validate`, `quality`) and `.github/workflows/quality.yml` now gate
  the contract (drift guard + redocly lint, via `.redocly.yaml`); generated code
  is excluded from the mutation/policy gates.
- ADR-0003 records that the contract-first pledge is now enforced.

## [0.21.0] - 2026-10-26

KLR: cost centres, cost units and accounting records (issue #20).

### Added
- migration `0015_klr.sql`: `cost_center`, `cost_unit`, append-only `accounting_record`.
- `internal/modules/klr`: domain, repo, audited service (`CreateCostCenter`,
  `CreateCostUnit`, `Post`, `Report`).
- HTTP: create cost centre / cost unit, post a record, read the report.

## [0.20.0] - 2026-10-26

Procurement: purchase orders with budget commitment (issue #18).

### Added
- migration `0014_procurement.sql`: `purchase_order` + `purchase_order_line`.
- `internal/modules/procurement`: domain, repo, audited service with the
  lifecycle `draft → approved → received` (+ `cancelled`), transitions guarded.
- **budget integration**: approval commits the gross to the order's budget
  position (via the budget module); cancelling an approved order releases it.
- HTTP: create/get/approve/receive/cancel purchase orders.

## [0.19.0] - 2026-10-26

Lookup catalogue (issue #17).

### Added
- migration `0013_lookup.sql`: one `lookup_value` catalogue keyed by
  `(namespace, code)` instead of 3,532 key tables.
- `internal/modules/lookup`: domain, repo, audited service (`Upsert`,
  `UpsertMany`, `Get`, `List`, `Seed`); `Seed` loads the standard code lists
  (UNTDID 1001/4461/5305, UNECE units, ISO 4217/3166).
- HTTP: list/get/upsert lookups and `POST /api/lookups/seed`.

## [0.18.0] - 2026-10-26

Budget: positions + postings with Mittelüberwachung (issue #15).

### Added
- migration `0012_budget.sql`: extends `budget_position` with `allocated_cents` +
  description/audit fields and adds append-only `budget_posting`.
- `internal/modules/budget`: domain, repo and audited service; the over-spend
  guard (`CheckAvailable`) rejects a commitment/actual beyond the available amount.
- HTTP: `POST /api/budget/positions`, `GET /api/budget/positions/{id}`,
  `POST /api/budget/positions/{id}/postings`, `GET /api/budget/positions/{id}/balance`.

## [0.17.0] - 2026-10-26

Inbound e-invoice: parse + validate a received XRechnung (issue #13).

### Added
- `internal/core/einvoice.ParseXRechnung`: parse a UBL 2.1 `Invoice` into our
  model and validate it with the same rules used for outbound invoices.
- `InvoiceService.ImportReceived` (audited, `einvoice.receive`) and
  `POST /api/e-invoices/received`.
- round-trip + parser error tests; integration and HTTP e2e tests.

## [0.16.0] - 2026-10-26

E-invoicing: XRechnung 3.0 (UBL 2.1) (issue #11).

### Added
- migration `0011_e_invoice.sql`: `e_invoice`, `e_invoice_party`,
  `e_invoice_line`, `e_invoice_vat` (EN16931-shaped, docs/08 §4).
- `internal/core/einvoice`: the model, EN16931 calculation rules and a
  deterministic **XRechnung 3.0 (UBL 2.1)** generator.
- `internal/modules/einvoice`: atomic persistence + audited service.
- HTTP: `POST /api/e-invoices`, `GET /api/e-invoices/{id}`,
  `GET /api/e-invoices/{id}/xrechnung`.
- golden file validated against the productive `UBL-Invoice-2.1.xsd`.

## [0.15.0] - 2026-10-26

S6 — foundation books (issue #9).

### Added
- migration `0010_foundation_book.sql`: `foundation_book` (one per foundation;
  `UNIQUE (foundation_id, reference)`) and append-only `foundation_booking`.
- `kasse.FoundationBook`/`FoundationBooking`/`FoundationBalance`, a repo and a
  service (`CreateBook`, `Book`, `Bookings`, `Balance`), audited on every write.
- HTTP: `POST /api/foundation-books`,
  `POST|GET /api/foundation-books/{id}/bookings`,
  `GET /api/foundation-books/{id}/balance`.
- **isolation test**: a booking on one foundation's book never appears in
  another's bookings or balance.

## [0.14.0] - 2026-10-26

CAMT.053 statement import (issue #7).

### Added
- **`internal/core/camt053`** — ISO 20022 CAMT.053 parser producing the shared
  parsed-statement shape; `internal/core/bankstmt` holds that neutral shape (the
  MT940 types are now aliases of it).
- `StatementService.Import` detects the format (XML → CAMT.053, else MT940).
- parser unit tests incl. a golden CAMT.053 sample; integration test importing a
  CAMT file and matching a paid order.
- `internal/core/camt053` added to `backend/mutation.json`.

### Changed
- `docs/legacy/kbs-statement-import.md` documents both formats.

## [0.13.0] - 2026-10-26

Statement-line detail: uncleared amount, counterparty, structured `:86:` (issue #5).

### Added
- migration `0009_statement_line_detail.sql`: `bank_statement_line` gains
  `uncleared_cents`, `end_to_end_ref`, `counterparty_name`, `counterparty_iban`,
  `counterparty_bic` (docs/06 §2.3).
- `internal/core/mt940`: parse the structured `:86:` fields in both conventions
  the productive parser supports — numbered (`?30` BIC, `?31` IBAN, `?32`/`?33`
  holder, `?34` purpose) and DK markers (`EREF+`, `SVWZ+`, `ABWA+`, `ABWE+`, …);
  `Line` gains `EndToEndRef` and the counterparty fields.

### Changed
- reconciliation matches on the client reference **or** the end-to-end reference.
- statement lines expose the new fields over HTTP.

## [0.12.0] - 2026-10-26

Split `bank_file` out of `bank_statement` (issue #3).

### Added
- migration `0008_bank_file.sql`: a `bank_file` entity (account, file hash;
  `UNIQUE (account_id, file_hash)`) owns the physical import; `bank_statement`
  references it and the unique key moves to `(bank_file_id, statement_number,
  page)`. Existing rows are backfilled. One file may now hold several statements.
- `kasse.BankFile`; repo `InsertBankFile`/`FindBankFile`/`GetBankFile`;
  `StatementService.BankFile`; `GET /api/bank-files/{id}`.
- `docs/06` §2.1 marked applied; `docs/legacy/kbs-statement-import.md` updated.

### Changed
- `StatementService.Import` is idempotent on the file (via `bank_file`) and
  audits one event per file; `Statement` carries `BankFileID` (was `FileHash`).

## [0.11.0] - 2026-10-26

Rename the own-account entity to `cash_account` (issue #1).

### Changed
- **Breaking (API + schema):** the university's own account entity is renamed
  `bank_account` -> `cash_account`, matching the reference implementation model
  (`docs/06` §2.2) and freeing `bank_account` for the counterparty concept.
  - migration `0007_cash_account.sql` renames the table and its unique
    constraint; the referencing FKs follow.
  - Go: `kasse.CashAccount`, `NewCashAccount`, `CreateCashAccount`,
    `GetCashAccount`, `ErrCashAccountRequired`.
  - HTTP: `POST /api/cash-accounts` (was `/api/bank-accounts`).

## [0.10.1] - 2026-10-26

Vendor model cross-walk.

### Added
- **`docs/06-vendor-model-crosswalk.md`** — a measured comparison of the productive
  the reference implementation finance schema (with its declared FKs and comments) against our
  canonical model and the legacy `kbs` tables. Confirms the Kasse design (our
  table names match the vendor's: `payment_order`, `bank_account`,
  `bankstatementline`, …) and records three refinements to schedule: split
  `bank_file` out of `bank_statement`; keep `cash_account` for the own account and
  `bank_account` for counterparties; add `uncleared_amount` + counterparty
  columns to `bank_statement_line`.

## [0.10.0] - 2026-10-26

Slice S4 — bank statement import (MT940) and reconciliation (Kasse).

### Added
- **Schema** (`migrations/0006_bank_statement.sql`): `bank_statement` (own
  identity: number, page, opening/closing balance — the legacy `kbs.mt940` god
  table repeated the header on every line) and `bank_statement_line` (one row per
  `:61:` with an explicit reconciliation `status` and `order_id`).
- **`internal/core/mt940`** — a SWIFT MT940 parser matching the reference implementation
  parser (`MT940Record`/`Field61`): tags `:20:/:25:/:28C:/:60F:/:61:/:86:/:62F:`,
  the `:61:` field layout, German decimal amounts, `C`/`D`/`RC`/`RD` indicators and
  the `009`/`059` return posting keys.
- **`internal/modules/kasse`** — `StatementService` + `StatementRepo`: parse,
  store, and reconcile each line against the **paid** orders (those in a payment
  run) by order reference, classifying `matched` / `returned` / `unmatched`;
  idempotent on the file hash.
- **HTTP**: `POST /api/bank-statements`, `GET /api/bank-statements/{id}`.
- **Tests**: parser unit tests incl. a golden MT940 sample; integration tests
  (import, reconciliation, idempotency, audit chain); HTTP e2e.
- **`docs/legacy/kbs-statement-import.md`** — S4 legacy findings and field mapping.
- `internal/core/mt940` added to `backend/mutation.json`.

### Quality
- All seven Go mutation targets above the 0.85 floor (`httpapi` 100 %, `kasse`
  99.6 %, `mt940` 92 %); policy-as-code green.

## [0.9.0] - 2026-10-26

Slice S3 — payment run and SEPA credit-transfer export (Kasse).

### Added
- **Schema** (`migrations/0005_sepa_export.sql`): `bank_account` (the debtor
  side, which the legacy `kbs` schema denormalised onto every row),
  `payment_run` (stores the produced `file_xml` and its `file_hash`), and
  `payment_run_item` with `UNIQUE(order_id)` as the double-payment guard.
- **`internal/core/sepa/pain001.go`** — deterministic generator for ISO 20022
  **pain.001.001.09** in the German DK/GBIC profile, matching the productive
  the reference implementation writer (`SEPAWriterCreditTransferVersion39`). Full validation
  (lengths, IBAN, BIC, currency, amounts) and XML escaping.
- **`internal/modules/kasse`** — `PaymentRunService` + `PaymentRunRepo`: build a
  run from approved orders for one debtor account, idempotent on its reference,
  audited; `BankAccount` validation.
- **HTTP**: `POST /api/bank-accounts`, `POST /api/payment-runs`,
  `GET /api/payment-runs/{id}`, `GET /api/payment-runs/{id}/file`.
- **Tests**: generator unit tests incl. a golden file
  (`internal/core/sepa/testdata/golden_pain001.xml`) **validated against the
  vendor's own `pain.001.001.09_GBIC_5.xsd`**; integration tests (run, idempotency,
  double-payment guard, audit chain); HTTP e2e.
- **`docs/legacy/kbs-sepa-export.md`** — S3 legacy findings and the field mapping.

### Quality
- `sepa`, `kasse` and `httpapi` at **100 % mutation efficacy** (gremlins,
  ADR-0008); all six target packages above the 0.85 floor; policy-as-code green.

## [0.8.0] - 2026-10-26

Productive the reference implementation source received and inventoried.

### Added
- **`docs/05-productive-source-inventory.md`** — measured inventory of the
  productive the reference implementation build handed over as three archives (full product WAR +
  two hotfixes). Records the module catalogue (341 modules), the vendor data
  model (`the reference schema jar`: 1 486 tables, 1 286 declared FKs,
  German column comments), the config catalogues (3 532 key tables; 1 762
  rights / 7 279 role-rights), the ERP Java sources (`the ERP *`, 1 481 files)
  and the payment-traffic layer (`the reference parser`, 1 651 files) with the
  shipped ISO 20022 XSDs (pain.001/008, camt.052/053/054).
- **`docs/open-questions.md`** — Q1 (which product) answered: the reference implementation is the
  vendor's unified successor; the dump is the legacy ERP. Q5–Q8 updated.

### Notes
- The archives are proprietary and remain uncommitted (AGENTS.md 1/2); they were
  extracted outside the repo. This entry is our analysis citing them.

## [0.7.1] - 2026-10-23

Architecture report: landscape pages, database ER models, more breathing room.

### Added
- **Database ER models in the report** (`tools/gen_erm.py`, `tools/erm.sh`,
  `make erm`). Two families, both generated, never hand-drawn:
  - **target canonical schema** — read *directly* from the SQL migrations
    (`backend/internal/platform/db/migrations/`), so every crow's foot is a
    declared `FOREIGN KEY`.
  - **legacy module schemas** (`sva4`, `mbs`, `cob`, `kbs`) — reconstructed from
    measured metadata (`schema/*.tsv`) and the naming conventions in
    `docs/03-schema-analysis.md`. The legacy schema has **zero** foreign keys, so
    every edge is labelled *inferred*.
- **`--no-erm`** flag and `make c4-pdf-no-erm` for a C4-only build.
- **`tools/test_erm.py`** — self-test guarding the ER generator (entity/FK counts
  stay in sync with the migrations; legacy edges stay labelled *inferred*). Wired
  into `tools/run_tests.sh` and `make test-tools`.

### Changed
- The report (`c4/export/campus-erp-c4-model.pdf`) is now **landscape A4** with
  one diagram per page, generous heading/figure spacing, and two parts —
  *Part I: C4 architecture model* and *Part II: Database ER models*. Previously
  the wide diagrams were shrunk into portrait pages and crowded against the text.

## [0.7.0] - 2026-10-23

Build, packaging and deployment automation.

### Added
- **Top-level `Makefile`** — one entry point for the C4 PDF, backend/frontend
  builds and tests, container images, the local stack, and deployment
  (`docs/build-and-deploy.md` documents every target).
- **C4 → PDF** (`tools/c4_pdf.sh`, `c4/pdf/template.tex`): `make c4-pdf` renders
  `c4/workspace.dsl` to a single PDF (`c4/export/campus-erp-c4-model.pdf`). The
  Structurizr CLI image cannot render images locally ("PNG/SVG export is not
  supported in this build" — that path needs the hosted API), so the script
  exports **PlantUML** and renders it with the `plantuml/plantuml` image, then
  compiles the report with **tectonic**. Both containers run as
  `--user $(id -u):$(id -g)` so the non-root Structurizr container can write.
- **`backend/Dockerfile`** — multi-stage; builds `api`, `worker` and
  `connectors` from one tree into one image (modular monolith, ADR-0004), role
  selected by the container command.
- **`frontend/Dockerfile` + `frontend/nginx.conf`** — Angular production build
  served by nginx, `/api` proxied to the backend on the same origin.
- **`docker-compose.yml` + `.env.example`** — local single-host stack
  (postgres, api, worker, connectors, web). `make up` / `make down`.
- **`deploy/helm/campus-erp`** — Helm chart for Kubernetes (ADR-0007): api,
  worker, connectors, web Deployments, api/web Services, optional Ingress and
  HPA, chart- or externally-managed DSN Secret. Validated with `helm lint` and
  `helm template`.
- **`frontend/proxy.conf.mjs`** — dev-server proxy for `make workon-frontend`
  (iterate on the SPA against a deployed backend; and the reverse via
  `make workon-backend`).

### Notes
- The backend image is verified end-to-end: `docker compose up` applies all four
  migrations and serves `/healthz`; the web container proxies `/api` to the api
  container.

## [0.6.1] - 2026-10-21

### Fixed
- **Flaky integration tests.** Test helpers built unique codes and business dates
  from a modulo-reduced timestamp (`time.Now().UnixNano() % 1000000`). The database
  persists between runs, so values repeated and collided on unique constraints —
  `TestDunningDuplicateLevelIsRejected` failed with a duplicate-level error, and the
  cash-book tests could collide on `business_date`. Uniqueness now comes from a
  database sequence (`testSeqNext`), which is the only source that is unique across
  parallel test packages *and* across runs. Business dates start at year 5000 so a
  freshly created sequence cannot collide with rows from earlier runs.
- Two consequences of that sequence, both hit and fixed while doing it:
  `CREATE SEQUENCE IF NOT EXISTS` is not atomic (parallel packages race on it — the
  same class of bug as the migration race), and a sequence starting at 1 re-issues
  values earlier runs already consumed.

## [0.6.0] - 2026-10-21

Slice S7: tax-office data and foundations.

### Added
- **Canonical schema** (`0004_tax_and_foundations.sql`): `vat_type`, `tax_office`,
  `foundation`.
- **VAT types** replacing `kbs.mwst_art`: integer basis-point rates (0..100 %),
  an enumerated `kind` (output/input) instead of the legacy `mw_typ` character, and
  no sign column — the legacy `mw_vorzeichen` stored `'+'`/`'-'` as text.
- **Exact VAT arithmetic**: `TaxOn`, `GrossFromNet`, `NetFromGross`, `TaxFromGross`,
  with `TaxFromGross` defined as the remainder so **net + tax equals the gross
  exactly** at every rate including 0 % and 100 %.
- **IBAN validation** (ISO 7064 mod-97) for tax-office bank details — the legacy
  `iban varchar(35)` accepted anything (`ErrIBANInvalid`).
- Tax-office and foundation records with real constraints and `(year, number)`
  uniqueness surfaced as a conflict.
- **HTTP API**: `POST /api/vat-types`, `GET /api/vat-types/{code}`,
  `POST /api/tax-offices`, `POST /api/foundations`.
- Tests: domain (VAT arithmetic incl. a rate/amount sweep proving the net+tax
  invariant, IBAN mod-97 with verified fixtures and length boundaries, character
  helper boundaries), service with fakes, Postgres integration, HTTP e2e.
- `docs/legacy/kbs-tax-and-foundations.md` — measured legacy findings behind the
  design.

## [0.5.0] - 2026-10-21

Slice S5: dunning ("Mahnwesen").

### Added
- **Canonical schema** (`0003_dunning.sql`): `dunning_level`, `dunning_band`,
  `dunning_notice`. Replaces the legacy `kbs.mahn`, which hard-codes five bands as
  the column groups `bis_betr1..5` / `mahn_geb1..5` / `prct1..5` — a sixth band
  would be a schema change.
- **Bands are rows**, ordered and validated: they must ascend by upper bound, and an
  open-ended band (bound 0) must be last (`ErrOpenBandNotLast`), so band selection
  is always defined.
- **Integer money arithmetic**: rates are basis points (1/100 %), the charge is
  `fee + round_half_up(amount × rate)`. No decimal floating point anywhere — the
  legacy stores `prct` as `numeric`.
- **Stored charges**: a notice records the charge it was issued with, so a later
  configuration change cannot rewrite history.
- Inactive levels are refused (`ErrLevelInactive`); a duplicate `(year, code)` is a
  conflict (`ErrLevelExists`).
- **HTTP API**: `POST /api/dunning/levels`, `GET /api/dunning/levels/{year}/{code}`,
  `POST /api/dunning/notices`.
- Tests: domain (band selection, rounding boundaries, validation), service with
  fakes, Postgres integration (including a direct-repository test proving the DB
  constraints hold and roll back), HTTP e2e.
- `docs/legacy/kbs-dunning.md` — measured legacy findings behind the design.

### Changed
- `roundDiv` lost an unreachable negative branch (amounts and basis points are
  non-negative by construction); dead code was flagged by mutation testing.

## [0.4.0] - 2026-10-21

Slice S2: payment orders ("Anordnungen") with an enforced four-eyes principle.

### Added
- **Canonical schema** (`0002_payment_order.sql`): `payment_recipient`,
  `budget_position`, `payment_order`, `payment_order_decision`. Replaces the legacy
  `kbs.ao` ~120-column god table, its 16 free-text slots, and the `kbs.zv_freigabe`
  row-clone "approval" (which recorded neither who released an order nor whether the
  releaser differed from the creator). Real foreign keys, integer cents, ISO-4217
  currency — the legacy schema has zero declared FKs.
- **Four-eyes rule**, enforced in the domain: `Approve` refuses when the approver is
  the creator *or* the submitter (`ErrFourEyes` → HTTP 403).
- **Workflow**: draft → submitted → approved / rejected / cancelled, with a mandatory
  reason on rejection and no cancellation of an approved order.
- **Decision trail**: one `payment_order_decision` row per transition, plus a
  hash-chained audit event — every decision is recorded in the current state, the
  history and the audit chain.
- **HTTP API**: `POST /api/payment-orders`, `GET /{id}`, `POST /{id}/{submit,approve,reject,cancel}`.
- **Optimistic locking** on transitions (`SaveOrderStatus` guarded by the status it
  was read at → `ErrOrderConflict`).
- Tests: domain (four-eyes, validation boundaries), service with fakes, Postgres
  integration, HTTP e2e, and a concurrent-migration regression test.
- `docs/legacy/kbs-payment-orders.md` — measured legacy findings behind the design.

### Fixed
- **Migrations are now advisory-locked.** `CREATE TABLE IF NOT EXISTS` is not
  race-free: two migrators starting at once (parallel test packages, or api and
  worker booting together) collided on `pg_type` with a duplicate-key error. Caught
  by the integration tests, covered by a 4-way concurrent regression test.

## [0.3.0] - 2026-10-21

Phase 1 (Kasse) vertical slice: platform foundations + cash-book day lifecycle.

### Added
- **Canonical schema** (`backend/internal/platform/db/migrations/`): append-only
  `audit_event`, `cash_book_day`, `cash_booking`. Database-per-tenant means no
  tenant columns (ADR-0002).
- **Migration runner**: embedded, versioned, idempotent; loadable from any `fs.FS`
  (`db.MigrationsFrom`).
- **Persistent audit store** (`audit.Store`): hash-chained append serialised with a
  PostgreSQL advisory lock, plus `Verify`.
- **Kasse module**: day lifecycle (`open`/`book`/`close`), domain rules, PostgreSQL
  repository, service emitting an audit event per write.
- **HTTP API** (`internal/httpapi`): health, audit verify, and the cash-book routes;
  handlers depend on interfaces so the transport layer is testable with stubs.
- **Tests**: Go unit + PostgreSQL integration + HTTP e2e (axe-free backend e2e via
  `httptest`), plus migration-loader tests.
- **CI**: `go` and `mutation-go` jobs now run against a Postgres service container.

### Fixed
- Audit timestamps are truncated to microseconds before hashing: `timestamptz`
  stores microseconds, so a nanosecond `time.Now()` did not survive the round-trip
  and `Verify` reported a broken chain (caught by the integration test).
- Integration tests no longer share mutable state: they use unique business dates
  and assert audit events per subject, because Go runs packages in parallel.
- gremlins' coverage cache is now isolated **per package** in `tools/run_tests.sh`
  and CI. A shared cache produced wrong, non-reproducible scores (one package
  reported 0 killed after another had run in the same container).
- `httpapi` handlers now share one `statusFor` error mapper (depend on interfaces,
  not the concrete service), which removed the untestable 500-branches and made
  `book`-on-a-closed-day a 409 (conflict) rather than 422.
- A duplicate business date now surfaces as `kasse.ErrDayExists` (409) instead of
  an opaque database error.

## [0.2.1] - 2026-10-20

### Changed
- Repository renamed `legacy-db-reverse-engineering` → `campus-erp`; Go module path
  updated to `github.com/Maximilian-Maag/campus-erp/backend`.
- `AGENTS.md`: corrected the Go mutation command — `./internal/...` is a vacuous pass;
  the runner enumerates packages from `backend/mutation.json`.

### Added
- `docs/plan/roadmap.md` — phase order (Kasse → KLR → Personal → Finanzen → Campus).
- `docs/plan/kbs-pilot.md` — Phase 1 (Kasse) delivery plan: scope, interfaces, target
  schema slice, ETL, slices S0–S8 with acceptance tests, risks, Definition of Done.

## [0.2.0] - 2026-10-20

Target-system design phase: decisions, requirements, C4 and a verified skeleton.

### Added
- **ADRs 0001–0008** (`docs/adr/`): new schema + ETL, database-per-tenant,
  Go + Angular contract-first, modular monolith, connectors isolation, audit &
  compliance, Kubernetes deployment, testing & mutation floor.
- **Requirements** (`docs/requirements.md`) rewritten for the target system:
  functional parity per module, interface parity, compliance, NFRs (15k users,
  peak load), tenancy, migration.
- **C4 model** as a Structurizr workspace (`c4/workspace.dsl`) modelling as-is and
  target; validated with the Structurizr CLI.
- **Backend skeleton** (Go, stdlib-only): `cmd/{api,worker,connectors}`,
  `internal/core/{audit,sepa}`, `internal/platform/config`, module placeholders.
  Unit tests pass; mutation 100% efficacy / 100% coverage on all three target
  packages (gremlins).
- **Frontend skeleton** (Angular 20): tenant resolution service, Karma config with
  a container-safe headless launcher, Playwright + axe e2e, Stryker config.
- **Contracts**: `contracts/openapi.yaml`.
- **Deployment placeholder**: `deploy/README.md`.
- **Quality wiring**: `tools/run_tests.sh`, `backend/mutation.json`,
  `.github/workflows/quality.yml`.

### Fixed
- `policy.json`: ignore Angular's JSONC `tsconfig*.json` (a checker false positive).
- Go mutation runner enumerates packages explicitly — gremlins silently reports
  "No results" for `./...` patterns, which would be a vacuous pass.
- sepa: extracted `isDigit`/`isLetter` helpers so the switch conditions are covered
  by mutation (mutator coverage 80% → 100%).

## [0.1.0] - 2026-10-20

### Added
- Initial reverse-engineering of the legacy university ERP sample dataset
  (`the sample dataset archive`).
- Module catalogue: the reference implementation (campus) vs the legacy university ERP, with module ids from
  `db_version` (`the HR module-GX`, `the finance module-UNIX`/`the finance module-GX`, `the cost-accounting module-GX`, `the cash-book module`).
- Database overview: 1,536 tables / 374 views / 38,453 columns across 4 module
  schemas; framework layer; no foreign keys.
- Schema analysis by module and domain cluster.
- Glossary of German domain terms.
- C4 model (context, container, component, code) as Markdown.
- Reproducible tooling: `scripts/restore.sh`, `scripts/analyze.sh`.
- Generated artefacts under `schema/` (DDL, inventories, keys).
- Policy as code: `tools/policy_check.py`, `policy.json`, pre-commit hook, CI.
