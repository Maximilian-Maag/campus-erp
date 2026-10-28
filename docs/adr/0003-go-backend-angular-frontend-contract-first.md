# ADR-0003 — Go backend, Angular frontend, contract-first

- Status: Accepted
- Date: 2026-10-20

## Context

The maintainer is familiar with .NET, Angular and C, and wants to learn Go on the
way. The stated goals are "very performant, simple to deploy, easy to understand".
Deployment is Kubernetes; the frontend must meet BITV 2.0 / WCAG (legally required
for German public universities).

## Decision

- **Backend: Go** — single static binary, fast startup, low memory, strong stdlib.
  Stack: stdlib `net/http` routing (or `chi`), **pgx + sqlc** (typed Go from
  explicit SQL), **River** (Postgres-backed job queue), **goose** (migrations),
  **huma** (OpenAPI-first handlers), `minio-go` for object storage.
- **Frontend: Angular** (SPA) — Angular 20, standalone components + signals,
  Angular Material + CDK for accessibility. State: signals + services (no NgRx
  unless it earns its place).
- **Contract-first:** the Go API is the source of truth for the OpenAPI spec; the
  Angular client is **generated** from it. CI runs contract tests so a spec change
  that breaks the client fails the build.
- **Auth:** OIDC Authorization Code + PKCE against the university IdP; the Go API
  validates the JWT and maps claims to roles. No local password store.
- **Tenant resolution:** hostname-based; no per-tenant build (per ADR-0002).

## Revision 2026-10-26 — contract-first is now enforced

The client-generation pledge above is implemented and gated in CI:

- `tools/gen_openapi.py` generates `contracts/openapi.yaml` from the Go route
  table + structs (49 paths / 51 operations / 58 schemas).
- `tools/api_contract.py` is the drift guard (Go routes ↔ spec, both ways) and
  generates the Angular client into `frontend/src/app/core/api/generated/`.
- `.github/workflows/quality.yml` fails if the committed client or spec is stale.
- Generated code is excluded from the mutation/policy gates (it is generated).

See `docs/15-api-contract.md`.

## Revision 2026-10-26 — OIDC implemented

Authentication is implemented (`internal/platform/auth`): RS256 JWTs are verified
against the IdP's JWKS and the authenticated subject becomes the audit actor. The
API requires a bearer token on `/api/*` when `ERP_OIDC_JWKS_URL` is set. See
`docs/17`.

## Revision 2026-10-28 — role authorisation implemented

Per-route role authorisation now backs authentication: `httpapi.authorize` /
`authorizeRead` gate every `/api/*` route with its owning module role, with
`erp-admin` as a bypass and `erp-reader` admitted on reads. The gate fails closed
without a principal and is a pass-through only when no verifier is configured. The
legacy `sv_rechte_sachb` fine-grained model projects onto these roles at the IdP;
the API enforces the module boundary. See `docs/17`.

## Consequences

- One artifact per backend service (copy binary + systemd/container) — satisfies
  "simple to deploy".
- Choosing an SPA makes **BITV/WCAG harder than server-rendered HTML**. Mitigation
  is mandatory: Angular Material/CDK, no custom widgets where avoidable, axe-core
  assertions in e2e, and a11y treated as a test gate. This is a real, budgeted cost.
- Two languages in the repo (Go + TypeScript) means two toolchains, two mutation
  runners, and a monorepo (see below).
- Generated client code must be excluded from mutation/hand-written-code policies
  (it is generated, not authored).

## Alternatives considered

- **Server-rendered HTML + htmx.** Best for a11y and simplicity, but the maintainer
  chose Angular and knows it; consistency with existing skills wins for an OSS
  project that must attract contributors. Rejected.
- **Rust / TypeScript / Python backend.** Rust is faster but slower to build and
  harder to contribute to; TS/Python deploy heavier. Go best matches the goals.
- **Separate backend/frontend repos.** A **monorepo** is chosen instead: one place
  for contracts, CI, policy-as-code and the C4 workspace; simpler for an OSS
  project to keep in sync.
