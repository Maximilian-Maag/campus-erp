# Changelog

All notable changes to this repository. Format: Keep a Changelog.

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
  `bank_account` -> `cash_account`, matching the productive HISinOne model
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
  HISinOne finance schema (with its declared FKs and comments) against our
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
- **`internal/core/mt940`** — a SWIFT MT940 parser matching the productive HISinOne
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
  HISinOne writer (`SEPAWriterCreditTransferVersion39`). Full validation
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

Productive HISinOne source received and inventoried.

### Added
- **`docs/05-productive-source-inventory.md`** — measured inventory of the
  productive HISinOne build handed over as three archives (full product WAR +
  two hotfixes). Records the module catalogue (341 modules), the vendor data
  model (`cs.sys.dbschema.hisinone.jar`: 1 486 tables, 1 286 declared FKs,
  German column comments), the config catalogues (3 532 key tables; 1 762
  rights / 7 279 role-rights), the ERP Java sources (`rm.erp.*`, 1 481 files)
  and the payment-traffic layer (`rm.fia.payments.datex`, 1 651 files) with the
  shipped ISO 20022 XSDs (pain.001/008, camt.052/053/054).
- **`docs/open-questions.md`** — Q1 (which product) answered: HISinOne is the
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
- Repository renamed `hisinone-db-reverse-engineering` → `campus-erp`; Go module path
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
- Initial reverse-engineering of the HIS-ERP Muster dump
  (`hisrm_muster_30_09_26.backup`).
- Module catalogue: HISinOne (campus) vs Hochschul-ERP, with module ids from
  `db_version` (`HISSVA-GX`, `HISMBS-UNIX`/`HISFSV-GX`, `HISCOB-GX`, `HISKBS`).
- Database overview: 1,536 tables / 374 views / 38,453 columns across 4 module
  schemas; framework layer; no foreign keys.
- Schema analysis by module and domain cluster.
- Glossary of German domain terms.
- C4 model (context, container, component, code) as Markdown.
- Reproducible tooling: `scripts/restore.sh`, `scripts/analyze.sh`.
- Generated artefacts under `schema/` (DDL, inventories, keys).
- Policy as code: `tools/policy_check.py`, `policy.json`, pre-commit hook, CI.
