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
    `HttpClient` service (`CampusErpApi`), one method per operation. A response that is not
    JSON (a stored file, the probe) is read with `responseType: 'text'` and the string overload
    of `HttpClient`: a `json` response would make Angular parse the body and an XML one would
    throw instead of arriving as a string.
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

`frontend/src/app/kasse/receivables.component.ts` — *Kasse: Offene Posten* (route
`/kasse/offene-posten`) — lists the open items a collection run selects from, by state (the
server's filter, `?state=`), and records one. The amount is typed in euros and stored in cents,
converted on the string so 0,01 € is exactly one cent; a comma is the decimal separator, as in
German. Whether an item may actually be collected — its mandate active, the currency matching —
is the API's decision at collection time, not the screen's.

`frontend/src/app/kasse/collection-runs.component.ts` — *Kasse: Einzüge* (route `/kasse/einzuege`)
— offers one of the university's own accounts, states a direct-debit run (reference, Gläubiger-ID,
Einzugsdatum, Sequenztyp, Local Instrument) and creates it one of two ways: from the open
receivables due by a date, or from mandates and amounts named by hand. Either way it shows the
pain.008 the run produced. What the run can gather stays the API's decision: the screen reports a
refusal (nothing due, a foreign currency, a due-by after the collection date) as it comes back
rather than re-deriving the rule. The hand-picked flow loads only the mandates the run's sequence
type can collect under (`?active=true&sequence_type=`, docs/21), so it does not offer a choice the
API would refuse; changing the sequence type reloads them and drops an item no longer collectable.

`frontend/src/app/kasse/payment-runs.component.ts` — *Kasse: Zahlungsläufe* (route
`/kasse/zahlungslaeufe`) — the credit-transfer counterpart of the collection run: offers one of
the university's own accounts, an execution date and the approved orders to gather (the server's
filter, `?status=approved`), sums what is chosen so the operator sees what the run will move, and
creates the run and shows the pain.001 it produced. The file can be handed to the bank channel,
which queues the work and answers with the job. Which orders a run may gather — approved ones,
each paid at most once — stays the API's decision: the screen offers what the server reports as
approved and reports a refusal (an unapproved order, an already-paid one, a duplicate reference)
as it comes back rather than re-deriving the rule.

`frontend/src/app/kasse/cash-accounts.component.ts` — *Kasse: Eigene Konten* (route
`/kasse/konten`) — lists the university's own bank accounts and enters one. It is the master data
every other Kasse screen offers before it can ask about one, and until it existed a fresh install
dead-ended on "es ist kein eigenes Konto angelegt" with no way out. The IBAN and BIC are the API's
to validate (ISO 7064 mod-97 and ISO 9362, shared with the rest of Kasse); the currency defaults
to EUR.

`frontend/src/app/kasse/dunning.component.ts` — *Kasse: Mahnwesen* (route
`/kasse/mahnwesen`) — lists the selected year's dunning levels with their bands, defines
one (the bands are entered as rows: a bound, a fee and a rate) and issues a notice against
a claim, showing the charge the API computed and stored. The year filter is the server's
(`?year=`). Amounts are typed in euros and a band's rate in percent; the level's year and
code travel from the picker, so a notice is only ever issued against a level that is on
screen — and when no level exists for the year, no notice form is offered. A notice's PDF
is linked by its document route rather than read through the generated client, which reads
that route as text; the link appears only when the notice carries a document.

`frontend/src/app/finanzen/foundations.component.ts` — *Finanzen: Stiftungen* (route
`/finanzen/stiftungen`) — finds a foundation, its books and a book's movements (`docs/29`). The
API could already create all three and read a book's movements and balance, but a foundation
could not be listed and its books could not be read: a fresh install could open a book and then
never reach it again. The screen reads in one direction, each level asked of the server for the
level above it (`GET /api/foundations/{id}/books`), so one foundation's books are never sifted
out of another's in the browser. The year filter is the server's (`?year=`), and a year the
server will not read is a failed load rather than an empty list. After a foundation is created
the list is read again — it is the server's answer to a filter the new row may not match — while
a new book or movement joins the list it was read for, and a book's balance is read again after
every movement rather than adjusted here, because the server derives it.

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

**Name the wire types.** A request body's nested structs — an object field or a slice of
them — must be a named type, not an anonymous `struct{...}`: the generator flattens an
anonymous struct into its parent, documenting top-level fields for a body the route does
not accept. The drift guard cannot see this (the route still matches), so it is a
contract defect that only shows up when a generated client is used. `createLevelRequest`
and `createOrderRequest` are the two places this has been hit; both now name their
nested types (`dunningBandRequest`, `recipientRequest`, `budgetPositionRequest`).

## Not yet done

- Twenty-two screens over five domains so far: Kasse (`/kasse/tag`, `/kasse/konten`,
  `/kasse/anordnungen`, `/kasse/zahlungslaeufe`, `/kasse/kontoauszuege`, `/kasse/offene-posten`,
  `/kasse/einzuege`, `/kasse/mahnwesen`, `/kasse/mahnlauf`, `/kasse/ruecklaeufer`,
  `/kasse/mandate`),
  Budget (`/budget/haushaltsstellen`, `/budget/mittelherkunft`),
  Beschaffung (`/procurement/bestellungen`, `/procurement/wareneingang`), E-Rechnung,
  KLR (`/klr/umlage`, `/klr/kostenarten`, `/klr/kostentraeger`, `/klr/verrechnungspreise`),
  Finanzen (`/finanzen/stiftungen`) and `/auswertungen` (the report catalogue). The rest get theirs as their
  phases land, each one starting from the read routes it needs — and the kasse/payments domain is
  where that bites: it had 32 operations and one list route, so nearly every screen there needs its
  `GET` built first.
- **No paging yet.** Every list route returns everything; a list big enough to need paging
  needs a page parameter on the route and on the screens that read it.
