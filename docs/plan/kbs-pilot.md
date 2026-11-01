# Phase 1 — Kasse (`kbs`) pilot

The first vertical slice: replace the HISKBS cash-book module end to end. Its
purpose is to **prove the platform** (tenancy, audit, ETL, interfaces, CI) on the
smallest, most self-contained legacy module before touching HR or the ledger.

## Status

| Slice | State |
|---|---|
| S0 platform foundations | **done for the pilot** — tenant DSN (`DATABASE_URL`), embedded migration runner (advisory-locked, safe under concurrent startup), append-only hash-chained audit store, CI with a Postgres service |
| S1 cash-book core | **done** — day lifecycle (open / book / close), PostgreSQL repository, HTTP API, audit event on every write |
| S2 payment orders + four-eyes | **done** — canonical `payment_order`/`payment_recipient`/`budget_position`, enforced four-eyes rule, decision trail, HTTP workflow |
| S3 payment run + SEPA export | **done** — `cash_account`/`payment_run`/`payment_run_item`; ISO 20022 **pain.001.001.09** (DK/GBIC), byte-identical golden file (XSD validation is wired via `golden.ValidateXSD` but skipped — the vendor schema is proprietary and not committed); idempotent on reference; double-payment guard |
| S4 statement import + reconciliation | **done** — `bank_file`/`bank_statement`/`bank_statement_line`; MT940 **and CAMT.053** parsers, lines reconciled against paid orders and classified `matched`/`returned`/`unmatched`; idempotent on the file |
| S5 dunning | **done** — `dunning_level`/`dunning_band`/`dunning_notice`, any number of ordered bands, integer basis-point rates, stored charges, HTTP workflow |
| S6 foundation books | **done** — `foundation_book`/`foundation_booking`; a separate book per foundation, append-only movements, derived balance; isolation tested |
| S7 tax-office data + foundations | **done** — `vat_type`, `tax_office`, `foundation`; exact VAT arithmetic, ISO 7064 mod-97 IBAN validation, HTTP workflow |
| S8 | **in progress** — ETL framework + pipelines (lookup catalogue, reference datasets) runnable from the API (`docs/19`); parallel run + cutover blocked on a production extract |

What exists is a **vertical slice, not parity**: the cash-book day lifecycle, the
payment-order workflow, the SEPA export, the statement import (MT940 + CAMT.053)
and foundation books. S8 (ETL + parallel run + cutover) is blocked on a
production extract.

Cash-book findings: `docs/legacy/kbs-cashbook.md` (S1) — the cash-book tables are
**structurally complete but empty** in the Muster (measured), so the cash-book ETL
dataset is deferred to a production extract, exactly as S2's payment orders are.

Legacy findings: `docs/legacy/kbs-payment-orders.md` (S2),
`docs/legacy/kbs-sepa-export.md` (S3), `docs/legacy/kbs-statement-import.md` (S4),
`docs/legacy/kbs-dunning.md` (S5),
`docs/legacy/kbs-tax-and-foundations.md` (S7).

## Why Kasse first

- **94 tables, 18 with data** — the smallest module; fastest to reach parity.
- **Self-contained** — few cross-module seams (`kbsans`, `stu_export` only).
- **One clean, well-standardised interface** — SEPA/MT940 (ISO 20022), so the
  `connectors` design (ADR-0005) is exercised on something standard.
- **Low blast radius** — cash booking is important but not payroll; a mistake here
  is recoverable in a way an HR error is not.

## Scope

### In scope (legacy processes)

| ID | Process | Legacy tables (representative) |
|---|---|---|
| FR-K1 | Cash book, bookings, collective bookings | `kass`, `schalterbuch`, `bu`, `buha`, `geing`, `dr`, `drprot` (`sb`/`sbprot` are the clerk master + change log, not the cash book — `docs/legacy/kbs-cashbook.md`) |
| FR-K2 | Payment orders | `ao`, `aoar`, `feao` |
| FR-K3 | Payment traffic, releases, four-eyes | `zawe`, `zawe_zusatz`, `zabe`, `zbu`, `zp_bankdaten`, `zp_stammdaten`, `zv_freigabe`, `transfrei` |
| FR-K4 | Statement reconciliation, return postings | `mt940`, `mt940ar`, `datmt940`, `rueckausz_*`, `rueckfibu_*`, `datrueckausz`, `datrueckfibu` |
| FR-K5 | Dunning | `mahn`, `mahnprot` |
| FR-K6 | Foundation cash books | `stift`, `stiftprot`, `tgas`, `titkto` |
| FR-K7 | Tax-office data, VAT types | `finanzamt`, `finanzamtprot`, `mwst_art` |

### Out of scope (this phase)

- Ledger postings themselves (that is `mbs`/Phase 4) — Kasse produces the return
  files that FiBu consumes; the *format* is in scope, the ledger is not.
- Cash-counting denominations beyond what `kass` requires.
- Anything not in `kbs`.

## Interfaces

| Interface | Direction | Detail | Test |
|---|---|---|---|
| SEPA credit transfer (pain.001) | out | payment runs — **implemented** | golden files |
| SEPA direct debit (pain.008) | out | mandates + collection run (`docs/20`–`22`) — **implemented** | golden files |
| Bank statement (MT940 / CAMT.052/053/054) | in | reconciliation — **implemented** | golden files |
| EBICS transport | both | via `connectors` — **planned** | sandbox/contract |
| Return postings to FiBu | out | `rueckfibu_*` format — **not implemented** | golden files |

Every interface: versioned contract, golden-file tests, idempotent re-processing.

## Target schema slice (canonical, new)

Indicative new tables (names TBD in the schema design): `cash_book`,
`cash_booking`, `collective_booking`, `payment_order`, `payment_run`,
`payment_route`, `cash_account`, `bank_statement`, `bank_statement_line`,
`reconciliation`, `dunning_case`, `foundation_book`, `tax_office_record`.

All writes go through the audit package; no hard deletes; every table is
tenant-scoped.

## ETL

`legacy kbs.* → canonical`, schema-driven (we hold the full DDL), re-runnable, and
reconciled by row counts + checksums + invariants (e.g. sum of bookings per day,
open dunning cases). No production extract exists yet — build against the Muster
plus synthetic fixtures generated from the schema; the first university's parallel
run is the acceptance test (ADR-0001).

## Slices (each independently shippable, each with tests)

| # | Slice | Acceptance test |
|---|---|---|
| S0 | Platform foundations (tenancy, auth, audit, db, CI) | a tenant can log in; every write emits an audit event; CI green |
| S1 | Cash book core (`kass`, `schalterbuch`) | integration test: create/book/close a cash day; audit chain verifies |
| S2 | Payment orders (`ao`, `aoar`) | four-eyes approval enforced; rejection path tested |
| S3 | Payment run + SEPA export (pain.001) | golden file byte-identical; idempotent re-run |
| S4 | Statement import (MT940) + reconciliation | golden file imported; matched/unmatched classified; return postings produced |
| S5 | Dunning (`mahn`) | dunning run produces cases + documents (PDF) |
| S6 | Foundations (`stift`) | separate book per foundation; isolation tested |
| S7 | Tax-office data (`finanzamt`, `mwst_art`) | VAT type mapping round-trips |
| S8 | ETL + parallel run + cutover | reconciliation passes; reversible cutover rehearsed |

## Non-functional targets for this phase

- API p95 < 300 ms at expected load (NFR-3).
- Peak (all users, semester start) absorbed by read replicas (NFR-2).
- Mutation ≥ 0.85 on all domain packages; unit + integration + e2e per slice.
- BITV/WCAG AA on every screen (CR-7), axe in e2e.

## Risks

| Risk | Mitigation |
|---|---|
| No production data → ETL rules unknown | schema-driven ETL + synthetic fixtures; parallel run is the acceptance gate |
| Bank file formats vary per university | golden-file suite per format variant; contract per bank |
| Cash is money → correctness is critical | reconciliation + immutable audit + four-eyes from S2 |
| Scope creep into the ledger | FiBu stays in Phase 4; only the return format is in scope here |

## Definition of Done (phase)

- All FR-K1..K7 processes implemented and audited.
- All interfaces live, golden-file tested, idempotent.
- Mutation ≥ 0.85; unit/integration/e2e green in CI.
- Parallel run reconciled against the legacy `kbs` schema at a real university.
- Cutover rehearsed and reversible; legacy module retired.
