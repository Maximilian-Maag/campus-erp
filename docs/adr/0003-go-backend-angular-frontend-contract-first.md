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
