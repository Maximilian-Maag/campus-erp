# Requirements (baseline — living document)

Status: **provisional**. This captures the requirements that can be derived from
the initial request and the evidence in the dump. The project's ultimate goal has
not been stated yet; §7 lists what must be decided before these become firm. See
`open-questions.md`.

Document conventions: **MUST / SHOULD / MAY** as in RFC 2119. Each requirement is
`FR-n` (functional) or `NFR-n` (non-functional).

## 1. Context

- A PostgreSQL dump (`hisrm_muster_30_09_26.backup`) of HIS eG's **Hochschul-ERP**
  was provided.
- The requester refers to the product as "HISinOne" but the dump contains the
  **ERP** modules (Personal/Finanzen/Kasse/KLR), not the campus-management
  modules. This discrepancy is the first thing to resolve (Q1).
- The immediate goal is to build **context** (module list + reverse-engineered
  schema + architecture docs) to support a subsequent project discussion.

## 2. Stakeholders

| Stakeholder | Interest |
|---|---|
| Project sponsor (user) | understanding of HIS structure to steer the project |
| Future developers | a data model they can build against |
| (TBD) domain experts | HR/finance/cashier process knowledge to validate semantics |

## 3. Scope (current phase)

In scope:
- HIS product/module catalogue.
- Reverse-engineering of the provided dump's schema.
- C4 architecture documentation of the as-is system.
- A private repository holding the above, with reproducible tooling.

Out of scope (so far): application code, data migration, integration
implementation, any modification of HIS software.

## 4. Functional requirements

| ID | Requirement |
|---|---|
| FR-1 | The repo MUST catalogue the modules of the HIS product family, distinguishing HISinOne (campus) from Hochschul-ERP, with the evidence for each. |
| FR-2 | The repo MUST document the database schema reverse-engineered from the dump: schemas, tables, keys, conventions, and the absence of foreign keys. |
| FR-3 | The repo MUST map tables to modules and to business domains (at least to the level of coherent clusters). |
| FR-4 | The repo MUST contain a C4 model (context, container, component, code) of the as-is system, with inferences clearly marked. |
| FR-5 | The repo MUST contain a glossary decoding the German domain terminology. |
| FR-6 | The reverse-engineering MUST be reproducible from the dump via scripts (MUST NOT depend on manual steps). |
| FR-7 | The repo MUST record open questions and assumptions explicitly. |
| FR-8 | The repository MUST be private. |

## 5. Non-functional requirements

| ID | Requirement |
|---|---|
| NFR-1 | Reproducibility: `scripts/restore.sh` + `scripts/analyze.sh` MUST regenerate every artefact under `schema/` from the dump. |
| NFR-2 | Privacy: the dump is Muster (sample) data, but no personal data MUST be introduced into the repo; the dump itself MUST NOT be committed. |
| NFR-3 | Provenance: every non-obvious claim MUST cite its evidence (source page, table name, or query). Inference MUST be marked. |
| NFR-4 | Policy-as-code: the repo MUST carry the house `tools/policy_check.py` + `policy.json` + hook + CI, per the user's standard. |
| NFR-5 | Documentation MUST be plain Markdown/Mermaid, renderable without proprietary tooling. |

## 6. Constraints & assumptions

- The dump is a **Muster/seed** database: many tables are empty; it is not
  production data.
- Only the database was provided — the application tier is inferred.
- The database was produced by PostgreSQL 17.9; a newer client (`pg_restore`)
  is required to read archive format 1.16 (the older `postgres:16` client
  refuses it — see `scripts/restore.sh`).
- The schema has **no foreign keys**; integrity is application-enforced.

## 7. To be decided in the project discussion

1. Target product: Hochschul-ERP (this dump) vs HISinOne campus (Q1).
2. The actual objective: understand / integrate / migrate / replace / report?
3. Required depth of the data model (cluster-level vs column-level dictionary).
4. Whether domain experts will validate semantics.
5. Deliverable format and audience.
