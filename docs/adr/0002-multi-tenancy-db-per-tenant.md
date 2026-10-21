# ADR-0002 — Multi-tenancy: database per tenant

- Status: Accepted
- Date: 2026-10-20

## Context

The product serves 1..N universities. Each tenant processes **HR and finance data**
— among the most sensitive categories under DSGVO — and German public-sector
finance is subject to GoBD/IDW separation expectations. The legacy system already
shows heavy per-institution configuration (hundreds of `k_*` catalogues,
`sv_konfig_*`) and **per-Bundesland** variance (`nrw_*`, `bw_lbv_*`, per-state
statistics and payroll formats).

Tenancy models considered: shared schema with `tenant_id`, schema-per-tenant,
database-per-tenant, cluster-per-tenant.

## Decision

**Database per tenant** on a shared PostgreSQL cluster (CloudNativePG). The API
resolves the tenant from the request (hostname / token claim) and selects a
per-tenant connection pool.

Regulatory variance (payroll, statistics, budget rules per Bundesland) is modelled
as **data + per-state Go packages selected by tenant configuration**, never as
branches in domain code.

## Consequences

- No cross-tenant query path exists — the strongest isolation available on one
  cluster, which is the right default for HR/finance.
- Migrations fan out across tenants; tenant onboarding = create DB + migrate +
  seed catalogues + (later) ETL import, fully automated.
- A connection-pool registry is required (tenant → pool via PgBouncer); pools are
  cached, never created per request.
- More databases to back up and monitor; mitigated by per-tenant automation and
  CloudNativePG.
- Per-tenant theming/config is served at runtime from an API endpoint; there is
  **no per-tenant build** (see ADR-0003).

## Alternatives considered

- **Shared schema + `tenant_id`.** Cheapest to operate, highest blast radius: one
  missing predicate leaks HR data across universities. Rejected.
- **Schema-per-tenant.** Middle ground, but Postgres catalog bloat and weaker
  isolation than separate databases, with little operational gain here. Rejected.
- **Cluster-per-tenant.** Strongest isolation, heaviest ops for an OSS product that
  must be "simple to deploy". Rejected.
