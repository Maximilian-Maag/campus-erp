# ADR-0004 — Modular monolith, not microservices

- Status: Accepted
- Date: 2026-10-20

## Context

The system covers several large domains (Personal, Finanzen/Sachmittel, Kasse,
KLR, later Campus). Load is 15k users per university with a **thundering-herd
peak** (all users at once at semester start / results release) and a few hundred
concurrent by day, 10-20 at night. Deployment target is Kubernetes; the stated
goal is "simple to deploy, easy to understand".

## Decision

Build a **modular monolith**: one deployable backend with internal packages per
domain module over a shared core (authz, audit, workflow, catalogues, reporting,
regulatory adapters). Modules are separated by package boundaries and interfaces,
not by network calls.

Run the same binary in three roles — `api`, `worker`, `connectors` — selected by
subcommand (see ADR-0005 for connectors).

## Consequences

- One artifact, one deployment, one set of migrations — meets the simplicity goal.
- The read peak is handled by scaling the stateless `api` horizontally plus read
  replicas (ADR-0007), not by splitting services.
- Long-running work (imports, exports, ETL, PDF) is isolated in `worker` so a
  nightly batch cannot starve the interactive API.
- Domain boundaries must be enforced in code (package structure, interface
  direction) — the compiler helps, but discipline is required to stop the monolith
  from becoming a mud ball.
- If a module ever needs independent scaling, the package boundary is already the
  extraction seam.

## Alternatives considered

- **Microservices.** Multiplies ops (N deployables, N pipelines, distributed
  tracing, eventual consistency) for no benefit at this scale, and directly
  contradicts "simple to deploy". Rejected.
- **Two-tier (fat DB, thin app).** The legacy pattern we are replacing. Rejected.
