# Architecture Decision Records

One file per decision. Format: Context / Decision / Consequences / Alternatives.
Status values: Proposed, Accepted, Superseded by NNNN.

| # | Title | Status |
|---|-------|--------|
| 0001 | Replace the HIS-ERP applications; migrate to a new canonical schema | Accepted |
| 0002 | Multi-tenancy: database per tenant | Accepted |
| 0003 | Go backend, Angular frontend, contract-first | Accepted |
| 0004 | Modular monolith, not microservices | Accepted |
| 0005 | Isolate native-dependency integrations in a separate connectors service | Accepted |
| 0006 | Audit and compliance (GoBD / IDW / DSGVO) as first-class constraints | Accepted |
| 0007 | Deployment target: Kubernetes | Accepted |
| 0008 | Testing and quality strategy (unit/integration/e2e, mutation floor 0.85) | Accepted |

Context for all decisions: reverse-engineering of the HIS-ERP Muster dump is in
`../01-module-catalogue.md` .. `../03-schema-analysis.md`.
