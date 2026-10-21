# Requirements — Target system (HIS-ERP replacement)

Status: **Accepted baseline** (2026-10-20). Supersedes the earlier context-gathering
baseline. Living document; changes require an ADR or a dated revision note.

Conventions: **MUST / SHOULD / MAY** (RFC 2119). IDs: `FR` functional, `IR`
interface, `CR` compliance, `NFR` non-functional, `TR` tenancy, `MR` migration.

## 1. Purpose

Build an **open-source, drop-in replacement** for the HIS eG Hochschul-ERP
(Personal, Finanzen/Sachmittel, Kasse, Kosten-/Leistungsrechnung) and, in a later
phase, the HISinOne campus modules. The product serves **1..N universities**, reuses
the existing **data** (not the schema), and targets "easy to understand, very
performant, simple to deploy".

## 2. Scope

In scope: all four ERP modules; the interfaces in §4; multi-tenant operation;
migration of legacy data.
Deferred: HISinOne campus modules (APP/STU/EXA/DOC) — same platform, later phase.
Out of scope: modifying or extending the HIS software; cloud-managed services.

## 3. Stakeholders

| Stakeholder | Interest |
|---|---|
| University HR / finance / cashier staff | day-to-day correctness and speed |
| Hochschulleitung / controlling | reporting and figures |
| Revision / auditors | audit trail, GoBD/IDW evidence |
| IT operations (Hochschul-RZ) | deployability, observability, backup |
| Data protection officer | DSGVO compliance |
| Open-source contributors | clarity, testability, contribution path |

## 4. Functional requirements (parity per module)

The replacement MUST cover the processes of each legacy module. (Process inventory
derived from the reverse-engineering; each item is verified against a real
university during rollout.)

### 4.1 Personal (`sva4`, HISSVA)

| ID | Requirement |
|---|---|
| FR-P1 | Manage positions and staff assignments (Stellen/Personal), incl. validity periods. |
| FR-P2 | Manage contracts, types of service and pay-scale assignment (Dienstart, Tarif, Besoldung). |
| FR-P3 | Produce payroll/besoldung notifications to the state pay office (LBV), per Bundesland. |
| FR-P4 | Travel-expense processing (Reisekosten, `rka_*`). |
| FR-P5 | Relocation/separation allowance (Trennungsgeld, `ter_*`). |
| FR-P6 | Benefits-in-kind (geldwerte Vorteile, `gvp_*`, `ggaa`). |
| FR-P7 | Working-time and leave management (`az_*`, `trg_*`, `ura`). |
| FR-P8 | Social-insurance reporting (Sozialversicherung, `sas*`, `shv`). |
| FR-P9 | Statistics delivery (Personalstatistik), per state. |

### 4.2 Finanzen / Sachmittel (`mbs`, HISMBS/HISFSV)

| ID | Requirement |
|---|---|
| FR-F1 | General ledger (FiBu): accounts, journals, debtors/creditors, period/year-end close. |
| FR-F2 | Budget management (Haushalt) incl. budget-consensus workflow (Konsens). |
| FR-F3 | Incoming-invoice register (Rechnungseingangsbuch) and document handling. |
| FR-F4 | Procurement/tendering under UVgO/VgV/VOB (requests, offers, orders, awards). |
| FR-F5 | Supplier and goods-receipt management. |
| FR-F6 | Property/lease management (GLM) and buildings/parcels. |
| FR-F7 | Inventory and asset management incl. depreciation (GWG/AfA). |
| FR-F8 | SEPA payment generation and statement import (see IR-2). |
| FR-F9 | Third-party funded projects and plans. |

### 4.3 Kasse (`kbs`, HISKBS)

| ID | Requirement |
|---|---|
| FR-K1 | Cash book (Schalterbuch), bookings, collective bookings. |
| FR-K2 | Payment orders (Ausgabeanordnung). |
| FR-K3 | Payment-traffic handling (Zahlweg), releases, four-eyes (Freigabe). |
| FR-K4 | Bank-statement reconciliation (MT940/CAMT) and return postings to FiBu. |
| FR-K5 | Dunning (Mahnwesen). |
| FR-K6 | Foundation cash books (Stiftungen). |
| FR-K7 | Tax-office data and VAT types. |

### 4.4 KLR (`cob`, HISCOB)

| ID | Requirement |
|---|---|
| FR-C1 | Budget structure (BUSA) incl. archived/imported versions and alternative hierarchies. |
| FR-C2 | Reference quantities (Bezugsgrößen) and recipients. |
| FR-C3 | Cost/activity lines and apportionment (Umlage: KPUM/UM*). |
| FR-C4 | Project and study-programme linkage. |
| FR-C5 | Third-party funding origins. |
| FR-C6 | Evaluations and report types. |

### 4.5 Cross-cutting functional

| ID | Requirement |
|---|---|
| FR-X1 | Role/permission model equivalent to the legacy `sv_rechte_sachb`, with four-eyes on financial postings. |
| FR-X2 | Reporting/print centre: Bescheide, certificates, Notenspiegel, Krankenkassenlisten, Modulhandbücher. |
| FR-X3 | Document management (upload/generate/store/retrieve), replaceable storage backend. |
| FR-X4 | Search across persons/documents (legacy had a dedicated search subsystem). |
| FR-X5 | Workflow/approval engine (legacy `wof`/`sv_vorgang*`). |

## 5. Interface requirements (external-system parity)

Formats are legally binding; the replacement MUST reproduce them exactly, not
invent new APIs. See `../c4` for the context diagram.

| ID | Interface | Direction | Notes |
|---|---|---|---|
| IR-1 | LBV / state pay office | out (+ returns in) | per-Bundesland formats; SFTP/file |
| IR-2 | Banks (SEPA pain.001/008, MT940/CAMT.053) | both | EBICS/file; via `connectors` |
| IR-3 | Finanzamt / ELSTER (ERiC) | out | native dep; via `connectors` |
| IR-4 | Statistics offices (Personal/Finanzstatistik) | out | per-state formats |
| IR-5 | Identity provider (Shibboleth/OIDC) | in | OIDC/SAML RP |
| IR-6 | DMS / document storage | both | own store + optional connector |
| IR-7 | HISinOne campus / QIS / SAP HCM | both/out | optional connectors |
| IR-8 | hochschulstart.de / DoSV | both | campus phase |

Every interface MUST have: a versioned format contract, golden-file tests, and an
idempotent, re-runnable processing path.

## 6. Compliance requirements

| ID | Requirement |
|---|---|
| CR-1 | GoBD-conformant, tamper-evident append-only audit trail. |
| CR-2 | No hard deletes on business data; reconstructable history. |
| CR-3 | IDW PS 880-relevant controls documented and evidenced for certification. |
| CR-4 | DSGVO: data-subject export (Auskunft) and pseudonymisation as first-class features. |
| CR-5 | Separation of duties / four-eyes on financial postings. |
| CR-6 | Reproducible document generation (template + data version archived). |
| CR-7 | BITV 2.0 / WCAG 2.1 AA for all UI (legally required). |
| CR-8 | Haushaltsrecht (LHO) rules supported per state. |

## 7. Non-functional requirements

| ID | Requirement |
|---|---|
| NFR-1 | **Scale**: support ≥15,000 users per university; **peak = all users concurrently**; a few hundred concurrent by day; 10–20 by night. |
| NFR-2 | **Peak read storm** (semester start, results release) MUST be absorbed by read replicas + caching, not degrade write paths. |
| NFR-3 | **Performance**: interactive page/API responses SHOULD be < 300 ms p95 at expected load. |
| NFR-4 | **Deployability**: one artifact per service; no manual steps; declarative k8s manifests/Helm. |
| NFR-5 | **Observability**: structured logs, metrics, traces, correlation IDs; audit stream separate from ops logs. |
| NFR-6 | **Availability**: target ≥ 99.5% during working hours; graceful degradation for batch. |
| NFR-7 | **Backup/restore**: per-tenant PITR; restore tested. |
| NFR-8 | **Security**: OIDC/SAML auth, least-privilege, secrets management, dependency scanning, no secrets in git. |
| NFR-9 | **Maintainability**: "easy to understand" — documented domain model, no exotic stack, contributor onboarding. |
| NFR-10 | **Portability**: on-prem, no mandatory cloud dependency. |
| NFR-11 | **i18n**: German primary, English secondary; translation-key parity enforced in CI. |

## 8. Tenancy & configuration

| ID | Requirement |
|---|---|
| TR-1 | Database per tenant on a shared cluster (ADR-0002). |
| TR-2 | Tenant resolved from hostname/token; no cross-tenant query path. |
| TR-3 | Per-tenant configuration and theming served at runtime; **no per-tenant build**. |
| TR-4 | Per-Bundesland regulatory behaviour as data + Go packages selected by tenant config. |
| TR-5 | Tenant onboarding fully automated (create DB, migrate, seed catalogues, import). |

## 9. Migration requirements

| ID | Requirement |
|---|---|
| MR-1 | Schema-driven ETL from the legacy schemas (we hold the full DDL). |
| MR-2 | Re-runnable and **reconcilable** (row counts, checksums, business invariants). |
| MR-3 | Parallel run per module before cutover; reversible cutover. |
| MR-4 | Synthetic fixtures generated from the legacy schema for integration tests (no production extract available). |
| MR-5 | ETL is isolated from domain code (own packages + queue). |

## 10. Constraints & assumptions

- No production data extract is available; the Muster dump is largely empty.
- The legacy DB has no foreign keys; integrity must be reconstructed.
- Certification is required — compliance work is not optional scope.
- Maintained by the project owner as an open-source project.

## 11. Traceability

| Requirement area | ADR |
|---|---|
| New schema + ETL | 0001 |
| Multi-tenancy | 0002 |
| Go + Angular + contract-first | 0003 |
| Modular monolith | 0004 |
| Connectors isolation | 0005 |
| Audit / compliance | 0006 |
| Kubernetes deployment | 0007 |
| Testing + mutation floor | 0008 |
