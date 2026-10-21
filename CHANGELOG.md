# Changelog

All notable changes to this repository. Format: Keep a Changelog.

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
