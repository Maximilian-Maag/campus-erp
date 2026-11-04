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
  document (49 paths, 51 operations, 58 schemas when this slice landed; more now). Structs referenced transitively
  are followed, so nothing dangles; qualified Go types (`kasse.Day`) become
  components with disambiguated names. It needs PyYAML to render, so it is a dev
  tool — but `--selftest` reads only the struct parser and also runs in the
  stdlib-only step. The parser accepts a trailing line comment on a field (a date
  field annotated with `// YYYY-MM-DD`); a field it dropped that way would be
  missing from the spec *and* the client at once, and the drift guard could not
  see it, because both are produced by the same parser. `make test` and CI run
  the self-test to pin that.
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

Three things the generator gets right that used to bite:

- A **qualified** field type (`budget.Position`) is resolved in the package it names. It used to
  fall through to `string`, so a response body's nested object reached the client as
  `position: string` — the budget position response was the one that showed it.
- A package name may be **split across directories** (`einvoice` is both the document model and
  the module that serves it), and both are scanned into that name, so a type is found wherever
  it lives.
- A route with a **raw body** (the received XRechnung) takes a `body: string` argument. It used
  to fall back to a `null` body, which made the route unreachable through the client.
- A route's **query parameters** are declared in the contract row as `(name, type, required)`
  triples; the client takes them as optional arguments and renders them through one `_query`
  helper. An omitted filter is not sent at all, rather than sent empty.

## The consumers

`frontend/src/app/kasse/cashbook-day.component.ts` — *Kasse: open a cash-book day* (route
`/kasse/tag`) — was the first screen of the rebuild and the proof the pipeline works: it
imports `CampusErpApi` from the generated client, builds the request through the generated
`OpenDayRequest` type, and renders the returned `Day`.

`frontend/src/app/budget/positions.component.ts` — *Budget: Mittelübersicht* (route
`/budget/haushaltsstellen`) — is the first read screen: it renders every position with Ansatz,
Verpflichtet, Ist and Verfügbar, and shows the loading, empty and failed states separately.
The available amount is the API's (`available_cents`), so the screen shows the figure the
Mittelüberwachung actually applies instead of recomputing the rule.

`frontend/src/app/procurement/orders.component.ts` — *Beschaffung: Bestellungen* (route
`/procurement/bestellungen`) — lists the orders and approves a draft, the step that commits
budget. Its status filter is the **server's own**: choosing a status asks the API for it
(`?status=`), so the screen never has to hold the whole list to narrow it.

`frontend/src/app/einvoice/invoices.component.ts` — *E-Rechnung: Eingangsrechnungen* (route
`/einvoice/rechnungen`) — reads a received XRechnung in, lists the invoices and matches one to
its order. Amounts and dates come from `core/format.ts` rather than from `Intl`: the same
`toLocaleDateString('de-DE')` renders "01.04.2026" in one browser and "1.4.2026" in another,
and a screen has to show the same figures wherever it runs.

`frontend/src/app/klr/allocation.component.ts` — *KLR: Umlage* (route `/klr/umlage`) — picks a
cost centre, shows its distribution keys, defines one and runs a period. The share is typed as
a percentage and stored as basis points, converted on the string rather than through a float so
0,01 % is exactly one basis point. The rule that a centre's shares must add up to 100 % stays
with the API.

`frontend/src/app/kasse/payment-orders.component.ts` — *Kasse: Zahlungsanordnungen* (route
`/kasse/anordnungen`) — lists the payment orders soonest due first and drives their workflow:
submit a draft, approve, reject (with a reason, entered as a second step) or cancel. The
workflow's rules stay with the API — the screen offers what a state allows and reports a refusal
as it comes back, so a four-eyes rejection reads as the refusal it is rather than being
pre-empted in TypeScript.

`frontend/src/app/kasse/bank-statements.component.ts` — *Kasse: Kontoauszüge* (route
`/kasse/kontoauszuege`) — lists the statements imported for one of the university's own accounts,
opens a statement's lines without leaving the page, and reads an MT940 file in by pasting its
content. Its account filter is the server's too (`?account_id=`). Whether an import is a *new*
file is the API's decision: importing the same file twice returns what was already stored.

Every screen has a link in the shell's navigation (`app.html`), and `e2e/app.e2e.spec.ts` fails
if the route table and those links disagree: a screen nobody can navigate to is not finished.

These are covered twice: a Karma spec drives the component against a mocked HTTP backend
(request, render, empty, failure, formatting, reload, and the action's own success and failure
paths), and a Playwright test drives the real browser against a stubbed API and runs **axe**
over the result — no violations, which is what CR-7/BITV 2.0 requires. The frontend mutation
gate (`npx stryker run`, break 85) covers them too.

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

- Seven screens over five domains so far: Kasse (`/kasse/tag`, `/kasse/anordnungen`,
  `/kasse/kontoauszuege`), Budget, Beschaffung, E-Rechnung and KLR. The rest get theirs as their
  phases land, each one starting from the read routes it needs — and the kasse/payments domain is
  where that bites: it had 32 operations and one list route, so nearly every screen there needs
  its `GET` built first.
- **No paging yet.** Every list route returns everything; a list big enough to need paging
  needs a page parameter on the route and on the screens that read it.
