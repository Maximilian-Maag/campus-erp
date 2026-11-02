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
(OIDC), LBV, ELSTER/ERiC, statistics, DMS, **SOAP service** surface,
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

## 7. Async jobs — the worker runtime (`internal/platform/jobs`)

Long-running work (imports, exports, ETL, report generation) runs through **River**
on Postgres (ADR-0007) — no extra broker. The runtime is `internal/platform/jobs`:

- `Migrate(ctx, pool)` applies River's schema — idempotent, called on every start.
  River owns its own tables, so they are not part of the app migrations and not in
  the ER model.
- `New(pool, datasets, importStatement, exportLedger)` builds a client with the
  standard workers registered; `StandardWorkers` registers three kinds: `etl`,
  `statement-import` and `ledger-export`.
- The `etl` kind (`EtlArgs{Dataset, Actor}`) runs a named dataset through the
  registry of §Platform-ETL (`docs/19`) and records the run on the audit stream,
  exactly as the API endpoint does. A dataset failure is returned so River retries.
- The `statement-import` kind (`StatementImportArgs{AccountID, Content, Actor}`)
  imports one bank-statement file; the file content travels with the job so a retry
  re-imports the same bytes. `StatementImportFunc` keeps the package free of a
  dependency on the cash-book module, as `DatasetRunner` does for ETL.
- The `ledger-export` kind (`LedgerExportArgs{From, To, Actor}`) renders a period's
  `cash-postings/v1` export (`docs/23`) and hands it over via `LedgerExportFunc`. The
  service refuses an export that does not balance against its bookings
  (`ErrLedgerUnbalanced`), so the job never delivers an unbalanced file.
- `EnqueueEtl` / `EnqueueStatementImport` / `EnqueueLedgerExport` are the enqueue
  seams the API role can use to hand work to the worker instead of running it inline.
- `cmd/worker` is the wiring: it opens the pool, migrates, builds the registry
  (legacy source pool when `ERP_LEGACY_DATABASE_URL` is set), starts the client and
  blocks until SIGINT/SIGTERM, then drains with a bounded stop.

## Verified

Go `vet`/`build`/`test`; gremlins per package ≥ 0.85 (`auth` 98 %, `tenant`
100 %, `interfaces` 100 %, `httpapi` 100 %, `jobs` 100 %); the tenant integration
test against two real databases; the authorisation matrix (`authz_test.go`); and the
worker integration test (`jobs`) which migrates River's schema, starts a client,
enqueues an `etl` job and waits for it to complete.

## 8. Interface artefacts - what each planned interface still needs

The registry (§4) marks nine interfaces `planned`. None is blocked on code: each is
blocked on an external artefact. This is the ledger, so the block is explicit rather
than implicit.

| Interface | Needs to proceed |
|---|---|
| Bank - EBICS channel | an EBICS contract, the agreed protocol version and a sandbox endpoint |
| State pay office (LBV) | the per-Bundesland file/communication specification and a test connection |
| ELSTER / ERiC | the ERiC library plus a test certificate and account |
| Statistics offices | the per-state export format (each office differs) |
| Document store (DMS) | the target DMS API definition and credentials |
| ERP SOAP services | a service catalogue / WSDL snapshot and a test instance |
| Student-loan portal | the loan service WSDL and test access |
| Card / cash terminal | the terminal protocol or its bridge documentation |
| Admission (DoSV) | the DoSV API specification and a test account |

Each entry is descoped explicitly if the artefact cannot be obtained; the registry
status then stays `planned` on purpose.

## Still open

- The tenant is resolved and used for the pool, but the audit `actor` does not yet
  record the tenant; wiring the tenant into audit events is a small follow-up.
- Interfaces are registered, not implemented: LBV, ELSTER, EBICS, the SOAP
  client and the rest remain `planned`.
- Three job kinds are wired: `etl`, `statement-import` and `ledger-export`. The
  remaining kinds (`sepa-export`, `statistics-export`, `report-render`) are not
  registered yet; each is a `JobArgs` + worker plus its enqueue call site.
- The worker's ledger hand-over is a directory (`ERP_LEDGER_OUTBOX_DIR`); object
  storage or SFTP replaces that function without touching the job kind.
