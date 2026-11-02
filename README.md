# university-ERP replacement

An open-source, multi-tenant, drop-in replacement for an established university ERP
(Personal, Finanzen/Sachmittel, Kasse, Kosten-/Leistungsrechnung) and, in a later
phase, campus management modules.

This repository holds the reverse-engineered understanding of the legacy system,
the architecture decisions for the replacement, the C4 model, and the code
skeleton.

## Goals

- **Drop-in replacement** for all four legacy ERP modules.
- **Open source**, serving **1..N universities**.
- **Easy to understand, very performant, simple to deploy.**
- **Certifiable** (GoBD / IDW / DSGVO / BITV) — compliance is a design constraint.

## Documentation map

| Path | Contents |
|---|---|
| `docs/01-module-catalogue.md` | module catalogue (product lines and modules) |
| `docs/02-database-overview.md` | legacy schemas, sizes, conventions, no-FK finding |
| `docs/03-schema-analysis.md` | reverse-engineered legacy data model |
| `docs/04-data-dictionary.md` | generated dictionary guide |
| `docs/glossary.md` | German domain terms |
| `docs/requirements.md` | **target-system requirements** (parity, interfaces, compliance, NFRs) |
| `docs/adr/` | **architecture decision records** 0001–0008 |
| `docs/plan/` | **delivery roadmap** + Phase 1 (Kasse) pilot plan |
| `docs/legacy/` | **measured legacy findings** per process (input to the canonical design) |
| `docs/open-questions.md` | open points |
| `c4/workspace.dsl` | **authoritative C4 model** (as-is + target), Structurizr |

## Repository layout

```
Makefile     single entry point: build / test / package / deploy (see below)
backend/     Go modular monolith (api / worker / connectors)  — ADR-0003, 0004, 0005
frontend/    Angular SPA                                        — ADR-0003
contracts/   OpenAPI spec (source of truth for the API client)
c4/          Structurizr workspace.dsl (as-is + target) + PDF template
deploy/      Helm chart (deploy/helm/campus-erp)                — ADR-0007
docker-compose.yml   local single-host stack
docs/        requirements, ADRs, legacy analysis
schema/      generated locally by scripts/analyze.sh (git-ignored)
scripts/     reproduce the legacy analysis
tools/       policy checker, quality runner, C4→PDF
```

## Architecture in one paragraph

A **modular monolith in Go** (one binary, three roles: `api`, `worker`,
`connectors`) serves an **Angular SPA** over a contract-first REST/JSON API. Data
lives in **PostgreSQL, one database per tenant**, on Kubernetes. Long-running work
(imports, exports, ETL, PDF) runs in `worker` on a Postgres-backed queue; native
dependencies (ELSTER/ERiC, EBICS) are isolated in `connectors`. Every state change
emits an **append-only, hash-chained audit event**. See `docs/adr/`.

## Build, test, deploy

One `Makefile` drives everything (needs only docker; the Go toolchain is optional):

```bash
make help           # all targets
make all            # C4 PDF + backend + frontend + container images
make test           # fast suite: Go + Angular + tooling + contract (no mutation)
make mutation       # mutation gates only: gremlins + stryker (floor 85)
make test-all       # both

make c4-pdf         # C4 model → c4/export/campus-erp-c4-model.pdf
make docker-images  # backend + frontend images
make up             # local stack (docker compose): postgres, api, worker, connectors, web

make deploy-backend && make workon-frontend   # deploy the API, iterate on the SPA
make deploy-frontend && make workon-backend   # deploy the SPA, iterate on the API

make helm-template  # render the k8s chart locally
make deploy-k8s     # push images + helm upgrade --install (ADR-0007)
```

Full reference: `docs/build-and-deploy.md`.

## Quality

- Unit, integration and e2e tests are mandatory; **mutation floor 0.85** (ADR-0008).
- Run everything: `tools/run_tests.sh`
- Policy as code: `tools/policy_check.py` (hook: `.githooks/pre-commit`; CI:
  `.github/workflows/policy.yml`).
- C4 validation: `c4/README.md`.

## Status

**Phase 1 (Kasse) in progress.** The design phase is complete (ADRs, requirements,
C4), and the first vertical slice is implemented and tested:

- platform foundations: tenant DSN, embedded migration runner, append-only
  hash-chained audit store, CI with a Postgres service
- cash-book day lifecycle (open / book / close) with a PostgreSQL repository and
  an HTTP API
- payment orders with an **enforced four-eyes rule** (the approver may not be the
  creator or the submitter), an append-only decision trail, and a hash-chained
  audit event per transition
- dunning levels with any number of ordered amount bands, integer basis-point rates
  and dunning notices that store the charge they were issued with
- VAT types with exact net/gross/tax arithmetic, tax-office master records with
  ISO 7064 mod-97 IBAN validation, and foundation references
- payment runs that export **SEPA credit transfers (ISO 20022 pain.001.001.09,
  DK/GBIC)**: the produced file is stored and hash-pinned, the run is idempotent
  on its reference, and an approved order can be paid at most once
- bank statement import (**MT940** and **CAMT.053**) with reconciliation: each
  line is matched against the paid orders and classified `matched` / `returned` /
  `unmatched`; the import is idempotent on the file
- foundation books: a separate, append-only book per foundation (S6), with a
  derived balance and tested isolation between foundations
- **e-invoicing**: emit **XRechnung 3.0 (UBL 2.1)** invoices (EN16931-shaped
  model, mandatory public-sector fields enforced, golden validated against the
  productive schema) and **receive** them (`POST /api/e-invoices/received`)
- **budget**: positions with append-only postings and Mittelüberwachung (over-spend guard)
- **procurement**: purchase orders whose approval commits budget to a position
  (draft → approved → received), cancelling releases it
- **lookups**: one seeded code catalogue keyed by (namespace, code), replacing the
  productive source's 3,532 key tables
- **KLR**: cost centres, cost units and append-only accounting records with a
  per-cost-centre report
- quality: Go unit + Postgres integration + HTTP e2e tests, mutation 100% on every
  Go target package, policy-as-code green

Shipped so far: the platform spine (auth, tenancy, readiness, interface registry,
async jobs), the Kasse pilot (cash book, payment orders + runs, SEPA credit transfer
and direct debit, MT940/CAMT.052/053/054 import, dunning, foundations, ETL), lookups,
budget, procurement, e-invoicing and KLR. To come: the Personal, Finanzen and Campus
phases, and the interfaces blocked on external specifications (EBICS, ELSTER, LBV,
statistics, DMS) - see `docs/plan/roadmap.md`.

The legacy analysis in `docs/` is the ETL input (ADR-0001). Its **generated**
artefacts are not committed: they are derived from a proprietary third-party database
dump and are reproduced locally with `scripts/analyze.sh`.

## What is not in this repository

- The third-party database dump the analysis is based on, and the artefacts generated
  from it (`schema/`, reproduced locally by `scripts/analyze.sh`).
- The vendor's XSDs, Java sources and JARs: tests that need them take a local path
  from an environment variable (`ERP_XSD_*`) and skip when it is absent.

`the reference implementation` and `the legacy university ERP` are products of the vendor. This project is an independent
reimplementation, not affiliated with or endorsed by them.

## License

Apache License 2.0 - see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

