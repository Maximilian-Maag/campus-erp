# campus-erp

An open-source, multi-tenant university ERP: a Go backend and an Angular SPA over
PostgreSQL, replacing a legacy system one module at a time — Kasse, Finanzen/Sachmittel,
Personal and Kosten-/Leistungsrechnung, with campus management to follow.

## Status

Phase 1 (Kasse) is shipped: cash book, payment orders with a four-eyes rule, SEPA payment
runs, MT940/CAMT statement import, dunning, foundations, ETL, budget, procurement,
e-invoicing (XRechnung 3.0) and KLR. Next: Personal, Finanzen and Campus — see
[`docs/plan/roadmap.md`](docs/plan/roadmap.md).

## Quick start

```bash
make dev-setup     # a fresh checkout → deps, hooks, .env, and the whole stack, login included
make deps          # Go modules + npm packages
make build         # backend binaries + the Angular bundle
make run           # postgres, api, worker, connectors, web on localhost
make test          # fast suite: Go, Angular, tooling, contract
```

`make dev-setup` is the one to run first: it wires what is missing and starts the stack with a
local Keycloak realm, so the login screen is live (`max.maag` / `kasse`).

Only docker is required; the Go and Node toolchains are optional
([`docs/build-and-deploy.md`](docs/build-and-deploy.md)).

## Architecture

A modular monolith in Go — one binary, three roles: `api`, `worker`, `connectors` — behind a
contract-first REST/JSON API ([`contracts/`](contracts/)). One database per tenant; long-running
work on a Postgres-backed queue; every state change emits an append-only, hash-chained audit
event. Kubernetes is the production target. Decisions:
[`docs/adr/`](docs/adr/), model: [`c4/workspace.dsl`](c4/workspace.dsl).

The interface is Angular with Angular Material (Material 3) and Transloco. One theme, one
global stylesheet, a shared frame and page header, and one vocabulary: every word on screen is a
message key, published in German (default) and English. See
[`docs/32-design-system-and-i18n.md`](docs/32-design-system-and-i18n.md).

## Images

Docker Hub, one tag per channel:

| tag | what it is | what moves it |
|---|---|---|
| `latest` | dev | every push to `dev` |
| `staging` | release candidate | every push to `staging` |
| `v0.136.0` | prod, built once and never moved | a `v*` tag |

`docker.io/maximilianmaag/campus-erp-backend`, `…-frontend`. Helm chart:
[`deploy/helm/campus-erp`](deploy/helm/campus-erp).

## Quality

Unit, integration and end-to-end tests are mandatory and the **mutation score floor is 0.85**
(ADR-0008). Style, hygiene and secret rules are executable
([`tools/policy_check.py`](tools/policy_check.py)), and the OpenAPI spec is checked against the
Go routes. See [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`AGENTS.md`](AGENTS.md).

## Documentation

- [`docs/requirements.md`](docs/requirements.md) — target-system requirements
- [`docs/17-platform-spine.md`](docs/17-platform-spine.md) — auth, tenancy, audit, jobs, interfaces
- [`docs/glossary.md`](docs/glossary.md) — the German domain terms
- [`docs/legacy/`](docs/legacy/) — the measured legacy analysis the design is built on

## What is not in this repository

The third-party database dump and the artefacts generated from it (reproduced locally with
`scripts/analyze.sh`), and the vendor's XSDs and JARs, which tests take from `ERP_XSD_*`
environment variables. This is an independent reimplementation, not affiliated with or endorsed
by the vendor.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
