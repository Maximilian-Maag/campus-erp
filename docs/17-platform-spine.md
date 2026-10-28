# 17 — Platform spine: auth, tenancy, readiness, interfaces

Phase 0 of the roadmap ("nothing ships without it") had four holes: no
authentication, no tenant resolution, a readiness probe that could not fail, and
no record of the external interfaces. This slice fills them. Each piece is
unit-tested and, where it needs a database, integration-tested.

## 1. Authentication & roles (`internal/platform/auth`)

Stdlib-only OIDC: the JWT is an RS256 token verified against the IdP's JWKS, so
the security-critical path pulls in no third-party dependency.

- `JWKVerifier` checks `alg`, the RSA signature, `exp`/`nbf` (with leeway) and
  `iss`/`aud`, then maps claims to a `Principal` (`sub`, `email`, `name`,
  `roles`).
- `KeySet` is an interface: `ParseJWKS` + `StaticKeySet` for tests, and
  `RemoteKeySet` (cached, refreshes on rotation, serves stale on a fetch error)
  for production.
- `StaticVerifier` exists for local development and must not be used in prod.

The HTTP layer (`httpapi/auth.go`) requires a bearer token on `/api/*` when a
verifier is configured; `/healthz`, `/readyz` stay open. The authenticated
subject becomes the audit `actor` (falling back to the dev `X-Actor` header only
when auth is disabled). Roles are coarse (`erp-kasse`, `erp-admin`, …); the
fine-grained legacy `sv_rechte_sachb` model maps onto them at the IdP.

Config: `ERP_OIDC_ISSUER`, `ERP_OIDC_AUDIENCE`, `ERP_OIDC_JWKS_URL`. With the
JWKS URL unset the API logs `auth: DISABLED`.

## 2. Tenancy — database per tenant (`internal/platform/tenant`)

ADR-0002 made concrete.

- `Resolve(host)` takes the first DNS label of the host as the tenant (the same
  rule the Angular app uses); anything that is not a valid label is rejected.
- `Registry` hands out one pool per tenant from a **DSN template** containing
  `{tenant}`, opened lazily and migrated once. The tenant name is validated
  before it is substituted, so a tenant cannot address another's database.
- `httpapi.Tenancy` is a multi-tenant front: it resolves the tenant per request,
  builds (and caches) that tenant's `Server`, and delegates. A malformed or
  unknown host is rejected with `400` before any tenant code runs; a tenant whose
  pool cannot be opened gets `503`.

`cmd/api` uses `Tenancy` when `ERP_TENANT_DSN` (the template) is set, and stays
single-tenant otherwise. `TestRegistryAgainstRealDatabases` creates two
databases, migrates them through the registry and asserts each has its own
schema.

## 3. Readiness (`/readyz`)

`/healthz` stays a pure liveness probe. `/readyz` pings the database (bounded by
`readyTimeout`) and returns `503` with the error envelope when it is not
reachable, so a load balancer stops routing to a process that cannot serve
(NFR-6).

## 4. Interface registry (`internal/platform/interfaces`)

The external interfaces are now data, not prose. `Standard()` seeds the registry
from requirements §5 and the bundle analysed in `docs/14`: the bank interfaces
(pain.001/008, MT940, CAMT.053, EBICS), e-invoicing (XRechnung in/out), the IdP
(OIDC), LBV, ELSTER/ERiC, statistics, DMS, the HISinOne **WDK SOAP** surface,
the KfW loan SOAP client and the BSOS chip-card terminal. Each entry has a
direction, a format, a requirement id and a status (`implemented` / `planned`).
`New` validates the set; `Render` produces a stable table pinned by a golden
file. This is the checklist the interface-parity work (`IR-1..IR-8`) draws from.

## 5. Test harness (`internal/core/golden`)

`golden.Compare`/`Assert` pins exact bytes with an `UPDATE_GOLDEN=1` update path,
and `golden.ValidateXSD` runs `xmllint` against a schema when one is supplied
(skipping cleanly otherwise). The SEPA and XRechnung tests now use it instead of
their own copies.

## 6. Authorisation — per-route role gates

`authenticate` proves *who* the caller is; `authorize` decides *what they may
do*. Every `/api/*` route is wrapped by `s.authorize(handler, roles…)` (writes) or
`s.authorizeRead(handler, module)` (reads), so the policy sits next to the route
rather than in a second table that can drift.

- A route is owned by its **module role** (`erp-kasse`, `erp-finanzen`, `erp-klr`,
  `erp-personal`); reads additionally admit `erp-reader`, and `erp-admin` may call
  any route. `authorize` folds in `erp-admin`, so a route lists only its own role.
- Shared reference data (the lookup catalogue) lists no role: any *authenticated*
  caller may read it, but only `erp-admin` may write it.
- The gate does not trust the context: reached without a principal it answers
  `401`, so a route mounted outside `authenticate` fails closed.
- With no verifier configured (local development) the gate is a pass-through,
  matching `authenticate`.
- The wrappers keep the `mux.HandleFunc("METHOD /path", …)` shape, so the contract
  drift guard (`tools/api_contract.py`) still parses the table unchanged.

The mapping is deliberately coarse — the fine-grained legacy `sv_rechte_sachb`
model is projected onto these roles at the IdP, so the API enforces only the
module boundary. `authz_test.go` pins the matrix (module role, reader, admin,
wrong module, no role, open route, missing/bad token).

## Verified

Go `vet`/`build`/`test`; gremlins per package ≥ 0.85 (`auth` 98 %, `tenant`
100 %, `interfaces` 100 %, `httpapi` 100 %); the tenant integration test against
two real databases; the authorisation matrix (`authz_test.go`).

## Still open

- The tenant is resolved and used for the pool, but the audit `actor` does not yet
  record the tenant; wiring the tenant into audit events is a small follow-up.
- Interfaces are registered, not implemented: LBV, ELSTER, EBICS, the WDK SOAP
  client and the rest remain `planned`.
