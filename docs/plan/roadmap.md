# Delivery roadmap

How the replacement is delivered: a strangler, module by module, on a shared
platform. Each phase ends with a **parallel run** against the legacy system and a
reversible cutover (ADR-0001).

## Phase 0 — Platform (prerequisite for every phase)

Not user-visible, but nothing ships without it:

- tenancy (database-per-tenant + pool registry) — ADR-0002
- auth (OIDC against the university IdP) + role/permission model
- **audit**: append-only, hash-chained event stream — ADR-0006
- ETL framework: legacy → canonical, re-runnable + reconcilable — framework + first pipeline (`docs/19`)
- deployment: CloudNativePG, PgBouncer, River, object storage — ADR-0007
- quality gates live in CI: unit, integration, e2e, mutation ≥ 0.85 — ADR-0008

## Phase order and rationale

| Phase | Module | Legacy schema | Why this order |
|---|---|---|---|
| 1 | **Kasse** | `kbs` (94 tables) | smallest, self-contained, one clean interface (SEPA/MT940); proves the platform end-to-end at low risk |
| 2 | **KLR** | `cob` (244) | read-mostly: proves legacy joins, hierarchy reconstruction and reporting with no write risk |
| 3 | **Personal** | `sva4` (636) | highest compliance weight (payroll, LBV, social insurance); do it once the platform is proven |
| 4 | **Finanzen/Sachmittel** | `mbs` (556) | largest and most heterogeneous (ledger, budget, procurement, property, inventory) |
| 5 | **Campus** | — (not in the dump) | highest user impact (APP/STU/EXA/DOC) but a separate product and a separate data source |

The order is deliberately lowest-risk-first. Campus has the biggest impact; it can
be **pulled forward** once Phase 0 and one ERP module are done, but it needs its own
dump — the provided one contains no campus schemas.

## Cross-cutting rule

A phase is not "done" until: feature parity for its processes, interfaces in
production (golden-file tested), audit coverage, mutation ≥ 0.85, and a successful
parallel run reconciled against legacy.

See `kbs-pilot.md` for the first phase in detail.
