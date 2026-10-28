# 15 — API contract: the first real feature of the rebuild

ADR-0003 promises a **contract-first** architecture: the Go API is the source of
truth for the OpenAPI document, and the Angular client is *generated* from it.
Until now that promise was aspirational — `contracts/openapi.yaml` documented 2
of 46 routes and there was **no generated client and no consumer**. This slice
makes the promise executable and gates it in CI.

## The problem, measured

| | before |
|---|---|
| Go routes (`backend/internal/httpapi/server.go`) | 46 |
| documented in `contracts/openapi.yaml` | 2 (plus a phantom `POST /audit`) |
| generated Angular client | none |
| frontend API usage | none (only `app.ts` + `tenant.ts`) |

The contract could rot silently: a handler could be added, renamed or deleted and
nothing noticed. That is the opposite of contract-first.

## The pipeline

```
backend/internal/httpapi/server.go   (Go route table)
backend/internal/httpapi/*.go        (request structs, json tags)
backend/internal/modules/*, core/*   (response structs)
        │
        ▼  tools/gen_openapi.py           (authoring; needs PyYAML)
contracts/openapi.yaml  ── generated, committed ──
        │
        ▼  tools/api_contract.py gen      (stdlib only)
frontend/src/app/core/api/generated/{models.ts,api.service.ts}
        │
        ▼
frontend/src/app/kasse/cashbook-day.component.ts   (a real consumer)
```

Two tools, two responsibilities:

- **`tools/gen_openapi.py`** — the *authoring* generator. It holds the explicit
  **contract table** (path → method → operationId → request/response type) and
  reads the Go structs (with their `json` tags) to emit a complete OpenAPI 3.1
  document: 49 paths, 51 operations, 58 schemas. Structs referenced transitively
  are followed, so nothing dangles; qualified Go types (`kasse.Day`) become
  components with disambiguated names. It needs PyYAML, so it is a dev tool.
- **`tools/api_contract.py`** — the *gate*. Stdlib-only (no YAML package, no JVM,
  no network) so CI can always run it:
  - `check` — the **drift guard**: every Go route must be documented *and* every
    documented route must exist in Go; every operation needs a unique
    `operationId` and a `responses` map; every `$ref` must resolve. Any drift
    fails the build.
  - `gen` — the **client generator**: TypeScript models + a typed Angular
    `HttpClient` service (`CampusErpApi`), one method per operation.
  - `selftest` — exercises the parser and generator on fixtures *and* on the
    committed spec.

## What the generated client looks like

```ts
getBankStatement(id: number | string): Observable<m.Statement> {
  return this.http.get<m.Statement>(this.base + `/api/bank-statements/${id}`);
}

importBankStatement(body: m.ImportStatementRequest): Observable<m.Statement[]> {
  return this.http.post<m.Statement[]>(this.base + `/api/bank-statements`, body);
}
```

Bodies and responses are typed from the Go structs; path parameters are typed;
`POST`s without a body pass `null` (Angular requires the argument).

## The first consumer

`frontend/src/app/kasse/cashbook-day.component.ts` — *Kasse: open a cash-book
day* — is the first screen of the rebuild and the proof the pipeline works: it
imports `CampusErpApi` from the generated client, builds the request through the
generated `OpenDayRequest` type, and renders the returned `Day`. It is covered by
a Karma spec (POSTs to `/api/cashbook/days`, renders the day, shows an error on
failure) and is reachable at route `/kasse/tag`.

## How it is enforced

- `tools/run_tests.sh` runs the contract self-test + drift guard.
- `make contracts-check` is part of `make quality`.
- `.github/workflows/quality.yml` has a `contract` job: it runs the guard, then
  regenerates the client and `git diff --exit-code`s the generated directory
  (the committed client must match the spec), and verifies the spec is in sync
  with the Go structs (`gen_openapi.py --check`).
- The generated directory is excluded from the mutation/policy gates
  (`frontend/stryker.conf.json`, `policy.json`): generated, not authored.

## Adding a route (the workflow)

1. Add the handler and register it in `httpapi/server.go`.
2. Add one row to the `CONTRACT` table in `tools/gen_openapi.py`.
3. `make contracts-gen` — regenerates the spec and the client.
4. Commit all three (Go, spec, generated client). The guard fails until the
   spec matches Go, so a missing row is caught immediately.

## Not yet done

- The client is generated but only one screen consumes it; the rest of the
  domains get screens as their phases land.
- Error responses are a shared `Problem` envelope in the spec, but the Go
  handlers still map errors per module (`statusFor`) and send `http.Error`
  plain text. Unifying them into the same JSON envelope is the next
  cross-cutting step (a follow-up slice).
