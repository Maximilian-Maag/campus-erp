# ADR-0006 — Audit and compliance (GoBD / IDW / DSGVO) as first-class constraints

- Status: Accepted
- Date: 2026-10-20

## Context

The system replaces an **IDW-audited, GoBD-relevant** ERP running German
public-sector finance and HR. Certification is a stated requirement. The legacy
system implements audit with per-table triggers (`<table>db`/`da`/`ib`/`ub`) and
`__account_history`; DSGVO features (Auskunft, Pseudonymisierung —
`data_pseudonymization`) are present in the dump.

Compliance cannot be retrofitted; it shapes the data model.

## Decision

Treat the following as non-negotiable design constraints:

1. **Append-only audit stream** — every state change emits an immutable audit event
   (who, what, when, before/after, correlation id), separate from the operational
   log. Tamper-evident (hash-chained) and archived to object storage with
   object-lock.
2. **No hard deletes** on business data — logical deletion + retention, so history
   is reconstructable.
3. **Reproducible documents** — generated Bescheide/certificates are archived with
   the template + data version that produced them.
4. **DSGVO by design** — data-subject export (Auskunft) and pseudonymisation are
   first-class operations, not ad-hoc scripts.
5. **Separation of duties** — role/permission model (successor to the legacy
   `sv_rechte_sachb`) supports four-eyes principles on financial postings.

## Consequences

- Audit is a core package used by every module; modules cannot bypass it.
- The audit table grows monotonically — partition + archive strategy required from
  day one.
- Immutability constrains modelling (corrections are compensating entries, as in
  real accounting).
- Certification audits need evidence: the audit stream and reproducibility
  guarantees are that evidence.

## Alternatives considered

- **Rely on database triggers (legacy approach).** Not portable across our schema
  changes and hard to certify as tamper-evident. Rejected.
- **Operational logs as audit.** Logs are mutable and incomplete; unacceptable for
  GoBD. Rejected.
