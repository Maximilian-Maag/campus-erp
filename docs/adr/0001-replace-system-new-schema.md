# ADR-0001 — Replace the HIS-ERP applications; migrate to a new canonical schema

- Status: Accepted
- Date: 2026-10-20

## Context

The goal is a drop-in replacement for all HIS-ERP modules (HISSVA, HISMBS/HISFSV,
HISCOB, HISKBS) and eventually the HISinOne campus modules. The provided dump is
the Hochschul-ERP Muster database: 1,536 tables across four module schemas, with
**no foreign keys** and application-enforced integrity, plus 374 views, 1,117
sequences, 3,601 functions and 1,439 triggers.

Two readings of "reuse the existing databases" were possible:
(A) keep the legacy schema as the system of record and replace only the app;
(B) re-model into a new schema and migrate the data;
(C) hybrid — legacy schema as system of record now, carve out a clean schema over time.

The project is an open-source product for 1..N universities, and the stated
quality goals are "easy to understand, very performant, simple to deploy".

## Decision

Adopt **(B): a new canonical schema**, with the legacy HIS database demoted to a
migration source and a parallel-run fallback. "Reuse the existing databases" means
reuse the **data**, not the schema.

The legacy schema is not carried forward as the domain model. A per-module
**anti-corruption / ETL layer** reads legacy tables and writes the new schema.

## Consequences

- The legacy 1,536-table, no-FK schema does not become the long-term model; the
  "easy to understand" goal is achievable because we own the new model.
- A migration programme is required: schema-driven ETL (we have the full DDL),
  synthetic fixtures for tests, and a **reconciliation** step (row counts,
  checksums, business invariants) between legacy and new during parallel run.
- **No production extract is currently available.** ETL correctness cannot be
  validated against reality yet; the first real university's parallel run is the
  acceptance test. ETL must therefore be re-runnable and reconcilable from day one.
- Legacy tables are read-only inputs. We do not write back to the legacy schema
  except during a bounded, reversible cutover.
- The trigger-based audit of the legacy system (per-table `<table>db` triggers,
  `__account_history`) does **not** carry over; the new schema needs its own
  audit design (ADR-0006).

## Alternatives considered

- **(A) Reuse the legacy schema.** Lowest migration risk, but permanently inherits
  a cryptic, integrity-free schema and contradicts the product goals. Rejected.
- **(C) Hybrid.** Pragmatic, but the user's goal is explicitly to "get rid of all
  of this system"; a clean target schema from the start avoids a second migration
  later. Rejected as the primary strategy, though a temporary parallel run is still
  required operationally.
