# Architecture Decision Records

One file per decision. Format: Context / Decision / Consequences / Alternatives.
Status values: Proposed, Accepted, Superseded by NNNN.

| # | Title | Status |
|---|-------|--------|
| 0001 | Replace the legacy university ERP applications; migrate to a new canonical schema | Accepted |
| 0002 | Multi-tenancy: database per tenant | Accepted |
| 0003 | Go backend, Angular frontend, contract-first | Accepted |
| 0004 | Modular monolith, not microservices | Accepted |
| 0005 | Isolate native-dependency integrations in a separate connectors service | Accepted |
| 0006 | Audit and compliance (GoBD / IDW / DSGVO) as first-class constraints | Accepted |
| 0007 | Deployment target: Kubernetes | Accepted |
| 0008 | Testing and quality strategy (unit/integration/e2e, mutation floor 0.85) | Accepted |
| 0009 | Render documents with an HTML→PDF service | Accepted |
| 0010 | A report is a definition plus rows | Accepted |
| 0011 | Artefacts go through one keyed store (directory or S3) | Accepted |

Context for all decisions: reverse-engineering of the legacy university ERP sample dataset is in
`../01-module-catalogue.md` .. `../03-schema-analysis.md`.
