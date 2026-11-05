# Changelog

All notable changes to this repository. Format: Keep a Changelog.

## [0.100.0] - 2026-11-05

The Mahnwesen gets its screen, the percent/basis-points pair moves where the other converters
live, and two request bodies were documented wrong.

### Added
- `frontend`: *Kasse: Mahnwesen* (`/kasse/mahnwesen`) — lists the selected year's dunning levels
  with their bands, defines one (bands entered as rows: a bound, a fee and a rate) and issues a
  notice against a claim, showing the charge the API computed and stored. The year filter is the
  server's; the level's year and code travel from the picker, so a notice is only ever issued
  against a level that is on screen. A notice's PDF is linked by its document route (the
  generated client reads that route as text), and only when the notice carries a document.

### Changed
- `shareToBasisPoints` / `basisPointsToShare` moved from the KLR allocation component into
  `core/format` and were renamed `percentToBasisPoints` / `basisPointsToPercent`. A second screen
  (the Mahnwesen) needs the pair, the cost-unit screen already imported it out of a sibling
  *component*, and importing a component for a pure conversion is the wrong coupling — the same
  reason `euroToCents` moved there. The conversion is still defined for 0…100 %, and the two
  directions are now tested as inverses.

### Fixed
- **Two request bodies were documented as flat fields the routes do not accept.**
  `POST /api/dunning/levels` takes `bands` as an array and `POST /api/payment-orders` takes
  `recipient` and `budget_position` as objects; the generator flattens a nested *anonymous*
  struct into its parent, so the spec (and the generated client) showed hoisted fields — a body
  the API rejects. The nested types are named now (`dunningBandRequest`, `recipientRequest`,
  `budgetPositionRequest`), and `docs/15` records the rule. The drift guard cannot catch this: the
  route still matches.

## [0.99.0] - 2026-11-05

The dunning levels can be listed, and the Mahnwesen gets the doc it never had.

### Added
- `GET /api/dunning/levels` — lists the levels with their bands, by year then code, `?year=` to
  narrow. The filter is the server's: no year lists every year, and a year nothing was entered for
  is an empty list, not a 404.
- `docs/28-dunning.md` — the dunning domain (level, bands, notice, the charge rule and the API),
  which until now was documented only in the pilot plan, the legacy findings and the document
  service.

## [0.98.0] - 2026-11-05

The university's own accounts get a screen — the master data every other Kasse screen needs.

### Added
- `frontend`: *Kasse: Eigene Konten* (`/kasse/konten`) — lists the university's own bank accounts
  and enters one (name, IBAN, BIC, currency). Every other Kasse screen offers one of these before
  it can ask about one, so without this a fresh install dead-ended on "es ist kein eigenes Konto
  angelegt". The IBAN and BIC are validated by the API (ISO 7064 mod-97, ISO 9362).

## [0.97.0] - 2026-11-05

The payment run gets its screen — the credit-transfer counterpart of the collection run.

### Added
- `frontend`: *Kasse: Zahlungsläufe* (`/kasse/zahlungslaeufe`) — offers one of the university's
  own accounts, an execution date and the approved payment orders to gather (the server's filter,
  `?status=approved`), sums what is chosen so the operator sees what the run will move, creates
  the run and shows the pain.001 it produced. The file can be handed to the bank channel, which
  queues the work and answers with the job. Which orders a run may gather — approved ones, each
  paid at most once — stays the API's decision: the screen reports a refusal as it comes back.

## [0.96.0] - 2026-11-05

The Einzüge screen can gather hand-picked items, not only the open receivables — and the mandate
list it picks from is built first.

### Added
- `GET /api/direct-debit/mandates` — lists the mandates, `?active=true` and `?sequence_type=` to
  narrow. The filters are the server's, so a screen that offers the mandates a run can collect
  under never holds every mandate to show a few.
- `frontend`: *Kasse: Einzüge* (`/kasse/einzuege`) now offers a second item source, "Aus gewählten
  Mandaten": pick a mandate and an amount (typed in euros, stored in cents) and build the run's
  items by hand, then create the run. It loads only the active mandates the run's sequence type
  can collect under, so a choice the API would refuse is not offered; changing the sequence type
  reloads them and drops a chosen item whose mandate is no longer collectable.

The explicit-items half of the collection run (`docs/22`) had no screen; it does now.

## [0.95.0] - 2026-11-04

The KLR thread closes: internal pricing gets its screen, and the money parser moves where both
directions of it live.

### Added
- `frontend`: *KLR: Verrechnungspreise* (`/klr/verrechnungspreise`) — lists the rates, defines one
  (the price is typed in euros and stored in cents) and charges a service, naming both ends, the
  quantity and the day it was rendered. A price needs one cost type and no second centre; the
  charge form is only offered once there are two centres to charge between, since a centre cannot
  charge itself.

### Changed
- `euroToCents` moved from the receivables component into `core/format`, next to `formatCents` it
  is the inverse of. A KLR screen needed it, and importing a Kasse component from the KLR would
  have been the wrong coupling; a second copy of the parsing rule is how the two directions drift
  apart.
- Removed a redundant `.trim()` before `Number(...)` in the charge form: `Number` already ignores
  surrounding whitespace, so the trim was dead code that no test could distinguish.

## [0.94.0] - 2026-11-04

Internal pricing: the KLR can price an internal service and charge it out, the last of the three
KLR engines.

### Added
- `service_rate` and `service_charge` (migration 0027), plus `POST/GET /api/klr/service-rates` and
  `POST /api/klr/service-charges`. A rate is the price of one unit of a cost type's service over a
  period; a charge prices a quantity at the rate **in force on the day the service was rendered**.
- `klr.ServiceRate`, `klr.ServiceCharge` and `klr.Service.ChargeService`, with audit events
  `service_rate.define` and `service_charge.record`. A charge's records carry the reference
  `LEI:<charge id>`.

### Notes
- **The amount is computed once and stored**, not re-derived from today's prices: a rate can change
  afterwards, and a charge that was made is a fact about the books. Where several rates cover the
  day, the most recently started one wins, which is what lets a rate change take effect without
  deleting the old one.
- The provider is relieved by exactly what the receiver is charged, so a charge moves cost between
  the two and changes no total — the same rule as the key-based engines.
- The vendor's `overhead_calc*` shape is unpublished, so the mapping stays data (docs/13). The KLR
  now has all three engines: distribution over centres by keys, attribution to cost units, and
  internal pricing.

## [0.93.0] - 2026-11-04

The Kostenträgerrechnung gets its screen, and the cost units get the list route a screen needs.

### Added
- `GET /api/klr/cost-units` — the cost units, by number. The KLR had a `POST` for them and no way
  to read them, so no screen could offer one to pick.
- `frontend`: *KLR: Kostenträgerrechnung* (`/klr/kostentraeger`) — picks a cost centre, shows its
  keys, defines one (the share is typed as a percentage and stored as basis points) and runs a
  period. What the shares must add up to stays with the API, as on the Umlage screen.

### Fixed
- The new screen shows dates through the shared `formatDay` ("01.04.2026"), not the ISO string the
  API sent. `formatDay` existed for exactly this and was unused.
- A reload now refreshes the keys of the centre that is already chosen. The first draft only
  loaded them while *picking* a centre, so a reload left the table showing the previous keys.

## [0.92.0] - 2026-11-04

The KLR's Kostenträgerrechnung: a cost centre's costs are attributed to its cost units, the other
half of the apportionment the centre-side engine already does.

### Added
- `cost_unit_key` and `cost_unit_run` (migration 0026), plus
  `POST/GET /api/klr/cost-centers/{id}/cost-unit-keys` and
  `POST /api/klr/cost-centers/{id}/cost-unit-keys/run`. A key says how much of a centre's costs a
  cost unit takes; a run books one cost on the source centre tagged with each unit that takes a
  share, and a revenue relieving the source by exactly what it handed out.
- `klr.CostUnitKey`, `klr.SplitUnitShares` and `klr.Service.AllocateToCostUnits`, with the share
  arithmetic shared with the centre-side split (`splitByBasisPoints`) rather than copied: the
  receivers differ, the rounding rule does not.
- Audit events `cost_unit_key.define` and `cost_unit_run.run`. A run's records carry the reference
  `KTU:<run id>`, distinct from the centre-side `UML:<run id>`.

### Notes
- **The source centre's balance does not move.** The tagged costs and the relief cancel, so
  attributing a centre's costs leaves its Saldo where it was while the unit carries them — a run
  still neither creates nor destroys cost. Records are append-only, so the cost *volume* rises.
- `docs/03` reads the legacy `kpum` / `um*` tables as "Umlage/Verrechnung", so the vendor's
  `cost_center_clearing` / `cost_unit_clearing` are taken to be the two sides of one distribution:
  the centre side is `allocation`, the unit side is this. The vendor's exact shape is unpublished,
  so the mapping stays data (docs/13).

## [0.91.0] - 2026-11-04

The KLR gets its Kostenträger view: the second stage of the cost accounting, next to the
Kostenstellen.

### Added
- `klr-cost-unit-overview` (*Kostenträgerübersicht*) — every cost unit with its Kosten, Erlöse and
  Saldo, plus a totals row, in the report catalogue both the API and the worker read from. The
  Kostenstellen say where a cost arose, the Kostenträger say what it was for. A snapshot like the
  Kostenstellenübersicht, so it takes no period, and its balance is derived per unit from the
  figures its row shows, so the totals cannot disagree with the rows above them.
- `klr.Repo.CostUnitFigures` reads every unit with what is booked on it in one query. A unit with
  no records is a row of zeroes rather than a missing row: "nothing is booked on it yet" is a fact
  about the unit, and leaving it out would read as if the unit did not exist. A record that names
  no cost unit is counted on no unit.

## [0.90.0] - 2026-11-04

The cost-type catalogue gets its screen.

### Added
- `frontend`: *KLR: Kostenarten* (`/klr/kostenarten`) — lists the catalogue, creates a cost type,
  shows the accounts of the chosen one and maps a new account, and looks an account up in the
  other direction. An empty catalogue still offers the form, so the first cost type can be created
  from the screen at all; an account nobody has mapped shows as an answer rather than a failure
  (the API's 404 is the answer). The trim of a typed account code and the choice of the first
  cost type are the screen's; what a code must look like and that an account belongs to at most one
  cost type stay the API's.

## [0.89.0] - 2026-11-04

The KLR gets its cost types (Kostenarten) and the bridge from the ledger's accounts to them.

### Added
- KLR **cost types** (`cost_type`) and the **account bridge** (`cost_type_account`), the piece
  `docs/13` listed as unpublished. The vendor's shape for it is not available, so the mapping is
  modelled as data: a cost type owns account codes, and a code is a plain string, so when the
  ledger side lands its codes are the ones the bridge already holds. An account belongs to exactly
  one cost type — mapping the same code to the same cost type again is idempotent, mapping it to a
  different one is refused. `POST/GET /api/klr/cost-types`,
  `POST/GET /api/klr/cost-types/{id}/accounts`, `GET /api/klr/accounts/{account}/cost-type`. The
  bridge's read side sits on the account because `/api/klr/cost-types/by-account/{account}` would
  be as specific as `/api/klr/cost-types/{id}/accounts` and the router refuses to guess between
  two equally specific patterns.
- `accounting_record.cost_type_id` (nullable): a record may name the cost type it belongs to. A
  record with none is valid — nobody has classified it yet.
- Audit events `cost_type.create` and `cost_type.map_account`.

## [0.88.0] - 2026-11-04

The KLR gets its per-centre view, in the report catalogue.

### Added
- `klr-cost-center-overview` (*Kostenstellenübersicht*) — every cost centre with its Kosten,
  Erlöse and Saldo, plus a totals row. It is a snapshot like the Mittelübersicht, so it takes no
  period, and the balance is derived per centre from the figures its row shows, so the totals
  cannot disagree with the rows above them. `GET /api/reports` offers it and the worker renders
  it to a PDF through the same path as the other reports — the catalogue is the one list both
  read, so the API and the worker cannot disagree about what exists.

## [0.87.0] - 2026-11-04

A run can be created from the UI, and the client can read a file it produces.

### Added
- `frontend`: *Kasse: Einzüge* (`/kasse/einzuege`) — offers one of the university's own accounts,
  states a direct-debit run (reference, Gläubiger-ID, Einzugsdatum, Fälligkeit bis, Sequenztyp,
  Local Instrument), creates it from the open receivables and shows the pain.008 it produced.
  What the run can gather (nothing due, a foreign currency, a due-by after the collection date)
  stays the API's decision — the screen reports a refusal as it comes back.

### Fixed
- **The generated client could not read a non-JSON response.** A route whose response is a stored
  file (or the probe) was fetched with `HttpClient`'s default `json` response type, so Angular
  parsed the body and an XML file threw instead of arriving as a string. Eight operations were
  affected — the payment-run and collection-run files, the dunning notice document, the e-invoice
  XRechnung, the artefacts download, `audit/verify`, `/healthz` and `/readyz` (the last three
  answer plain text, so the client had been failing on them too). A non-JSON response is now read
  with `responseType: 'text'` and the string overload of `HttpClient`, and `api_contract.py
  selftest` pins that.

## [0.86.0] - 2026-11-04

The open receivables reach a screen.

### Added
- `frontend`: *Kasse: Offene Posten* (`/kasse/offene-posten`) — lists the open items a collection
  run selects from, filtered by state on the server (`?state=`, so the screen shows one state
  rather than holding every item and sifting), and records one. An amount is typed in euros and
  stored in cents, converted on the string rather than through a float so 0,01 € is exactly one
  cent, with a comma as the decimal separator and points grouping thousands. Whether an item may
  be collected (its mandate active, the currency matching) stays the API's decision at collection
  time; the screen does not re-derive it.

## [0.85.0] - 2026-11-04

The generated client could not name a field that carried a comment.

### Fixed
- **A Go request field with a trailing comment was dropped from the spec and the generated
  client.** `tools/gen_openapi.py` matched a struct field line only when nothing followed the
  `json` tag, so a collection date written as `` `json:"collection_date"` // YYYY-MM-DD `` — and
  the same shape on the mandate's `signature_date`, a receivable's `due_date` and the collection
  run's `local_instrument` — never reached the contract. The client could not set a collection
  date or a mandate's signature date at all, and neither could a test. The drift guard could not
  see it: the spec and the client were produced by the same parser, so they agreed with each
  other while both disagreed with the Go structs. The parser now reads a trailing line comment,
  the six fields reappear in the spec and the client, and `gen_openapi.py --selftest` (run by
  `make test` and CI) pins the case.

## [0.84.0] - 2026-11-04

A direct-debit collection run can select its own items — the open receivables.

### Added
- `receivable` (migration 0024): an open item (offener Posten) under a mandate, with a soft state
  (`open`/`collected`/`cancelled`). It carries no debtor of its own — the mandate supplies the
  debtor and the authority to collect, so the two cannot disagree — and a collected item is linked
  to the run that took it, never deleted.
- `POST`/`GET /api/receivables` and `GET /api/receivables/{id}` (role `kasse`): record and read the
  open items, by due date, optionally narrowed to one state.
- `POST /api/direct-debit/runs/from-receivables` (role `kasse`): a collection run that selects the
  open items due on or before a `due_by` date, under active mandates matching the run's sequence
  type. An item not yet due, or under a retired or differently-sequenced mandate, cannot be
  gathered and is left open rather than failed on. A mandate's items are collected as one amount —
  the pain.008 file allows one collection per mandate — and the selected items are marked
  `collected` and linked to the run in the same transaction that stores it, so a stored run and its
  claims cannot disagree.

### Changed
- The collection run's items are no longer only what the caller hands it; the run can select them
  from what is owed. Idempotency is checked before selection, so re-issuing a reference whose items
  are already collected returns the stored run instead of finding nothing left to collect.

### Fixed
- An open item in another currency than the collecting account's is refused
  (`ErrReceivableCurrencyMismatch`) rather than collected at a silent rate of one.

## [0.83.0] - 2026-11-04

Bank statements reach a screen, and the two reads behind it.

### Added
- `GET /api/cash-accounts` (role `kasse`): the university's own accounts by name. A screen that
  offers an account has to be able to read them; the kasse had only the create route.
- `GET /api/bank-statements` (role `kasse`): the imported statements, newest closing date first,
  with the account filter as a query parameter. `kasse.StatementSummary` +
  `StatementRepo.Statements`.
- `frontend`: *Kasse: Kontoauszüge* (`/kasse/kontoauszuege`) — lists one account's statements,
  opens a statement's lines in place, and reads an MT940 file in by pasting its content.

### Fixed
- **The statement list broke on a row it could not date.** `closing_date` and `opening_date` are
  nullable, and the list scanned them into `time.Time`: one undated statement — the bank may send
  one, and the ETL path can store one — failed the *whole answer* with a 500, not just that row.
  They are read the way the single-statement read reads them, and an undated statement sorts last
  rather than leading a newest-first list.
- The account filter's parameter was typed as an `integer` by its own comparison, so an account id
  past the 32-bit range was refused instead of simply matching nothing.

### Changed
- **Every screen is now linked from the shell's navigation, and a test enforces it.** The
  navigation lives in `app.html` and three screens landed without a link — reachable only by
  typing a URL. `e2e/app.e2e.spec.ts` now fails if the route table and those links disagree.

## [0.82.0] - 2026-11-03

Payment orders reach a screen, and the workflow they drive.

### Added
- `GET /api/payment-orders` (role `kasse`): the payment orders, soonest due first — the order an
  operator works through them in — with the status filter as a query parameter, so the screen
  never has to hold every order to work on part of them. `kasse.OrderSummary` +
  `OrderRepo.ListOrders`.
- `frontend`: *Kasse: Zahlungsanordnungen* (`/kasse/anordnungen`) — lists the orders and drives
  the workflow: submit a draft, approve, reject (with a reason, entered as a second step) or
  cancel. The screen offers what a state allows and reports a refusal as it comes back; the
  workflow's rules (only a draft is submittable, approval is four-eyes) stay with the API.

### Fixed
- The generated client's `OrderSummary` collided once kasse gained one too, and the generator
  disambiguates by package: procurement's became `ProcurementOrderSummary`. The procurement
  screen and its spec were updated, which is the whole of the change — the generator picked the
  name, so nothing hand-written decides it.

## [0.81.0] - 2026-11-03

The Umlage screen, and the cost-centre list behind it.

### Added
- `GET /api/klr/cost-centers` (role `klr`): every cost centre by number. A screen that offers a
  centre has to be able to read them; the KLR had only the create route.
- `frontend`: *Umlage* (`/klr/umlage`) — pick a cost centre, see its distribution keys, define
  one, and run a period's distribution. The share is typed as a percentage and stored as basis
  points (`shareToBasisPoints` is exact, on the string, so 0,01 % is one basis point and not
  something that rounds to it). The rule that a centre's shares must add up to 100 % stays with
  the API: the screen does not re-derive a backend guard.

### Notes
- The screen reports a refused run as it is (`Konnte die Umlage nicht ausführen.`) and keeps the
  keys visible — a run that was refused changed nothing, so hiding the list would suggest
  otherwise.

## [0.80.0] - 2026-11-03

The KLR distributes: a cost centre's costs reach its receivers by keys, exactly and once.

### Added
- The KLR distribution engine (migration 0022, `internal/modules/klr/allocation.go`): a cost
  centre's period costs are handed to its receivers in the shares its keys give, and the source
  is relieved by the same amount through a `revenue`. `POST /api/klr/cost-centers/{id}/allocations`
  defines a key, `GET …/allocations` lists them, `POST …/allocations/run` distributes a period.
- A distribution key is **data** — source centre, receiving centre, share in basis points,
  validity — because the vendor's `overhead_calc*`/`allocation*` shape is not published
  (`docs/13`), the same reasoning as the per-state statistics layouts (ADR-0012).
- `allocation_run` records each run, and the records it wrote carry its reference (`UML:<run>`),
  so a distributed cost is traceable in the report.
- `Run` and `Allocation` on the API; audit events `allocation.define` and `allocation.run`.

### Changed
- KLR records carry a **business date** (migration 0023): the period a record belongs to, which
  is not the day it was entered — the month's costs are booked after it ends, and that is the
  period they must be distributed over. `POST …/records` accepts an optional `business_date`
  and defaults to today.

### Notes
- The split is exact: each receiver gets its share rounded down and the last takes the
  remainder, so no cent is created or lost, and a share that rounds to nothing books nothing
  (a record's amount is always positive).
- One run per centre and period, enforced by an **exclusion constraint over the period**: an
  overlapping window is refused, not merely an identical one, because two runs over the same
  days would hand out the same costs twice. The run and its records are written in one
  transaction, and a refused run leaves the books untouched.
- The sum of balances across the centres is unchanged by a run — it moves cost, it neither
  creates nor destroys any. The cost *volume* does rise by the allocated amount, since the
  receivers' records are new and the source's are not removed: records are append-only.
- Receivers are cost centres. Distributing onto a cost *unit* (Kostenträger) is a separate step
  and no column for it was added; `docs/13` says why.

## [0.79.0] - 2026-11-03

A stored invoice knows which way it goes.

### Added
- `e_invoice.direction` (migration 0021) and `Invoice.Direction`: `inbound` for a received
  Eingangsrechnung, `outbound` for one this system issued. `ImportReceived` stores the former,
  `Create` the latter, and `GET /api/e-invoices` reports it.
- `einvoice.ErrNotInbound` (422): only a received invoice can be matched to a purchase order.
  The screen shows the direction and offers the match action only where one is possible.

### Notes
- The column arrived after the first invoices did, so the migration backfills it from the audit
  stream — the rows whose trail shows `einvoice.receive` become `inbound` rather than being
  guessed at, and everything else stays `outbound`.

## [0.78.0] - 2026-11-03

Query parameters in the generated client, so a screen can use a route's filter.

### Added
- Contract rows may declare query parameters as `(name, type, required)` triples. The OpenAPI
  spec emits them, and the generated client takes them as optional arguments
  (`listPurchaseOrders(status?: string)`), rendering one `_query` helper that drops anything
  omitted or empty.

### Changed
- The orders screen filters **on the server** now: choosing a status asks the API for it
  (`?status=`), instead of sifting the list it happens to hold. The end-to-end test asserts the
  parameter really travels.

## [0.77.0] - 2026-11-03

The e-invoice screen, the invoice list, and the match recorded on the invoice.

### Added
- `e_invoice.matched_order_id` (migration 0020) and `Invoice.MatchedOrderID`: a matched invoice
  records the order it was bound to. The audit stream alone could not serve a screen — that is
  the trail, not readable state — and a second match is now refused with `ErrAlreadyMatched`
  (409) by the invoice rather than by the order's state.
- `GET /api/e-invoices` (role `finanzen`) and `InvoiceSummary`; `InvoiceRepository.List` /
  `InvoiceService.Invoices`.
- `frontend`: *E-Rechnung — Eingangsrechnungen* (`/einvoice/rechnungen`) — reads a received
  XRechnung in, lists the invoices with their match state, and matches one to its order.
- `frontend/src/app/core/format.ts`: the shared, deterministic amount and date formatting the
  screens use.

### Fixed
- The generated client could not drive a route with a **raw body**: it emitted a `null` body,
  so `POST /api/e-invoices/received` was unreachable through `CampusErpApi`. Such a route now
  takes a `body: string` argument.
- The generator scanned one directory per package name, so a type living in `modules/einvoice`
  (whose name is shared with `core/einvoice`) fell back to a free-form object. Both directories
  are scanned into the name now.
- The screens formatted amounts and dates with `Intl`, which is not stable across browsers: the
  same `toLocaleDateString('de-DE')` gives "01.04.2026" here and "1.4.2026" there. Formatting
  moved to one shared helper and a test caught it.

## [0.76.0] - 2026-11-03

The procurement screen, and the order list behind it.

### Added
- `GET /api/purchase-orders` (role `finanzen`), optionally narrowed with `?status=`. The
  statuses come from the domain (`procurement.Statuses`), and an unknown one is a 422 rather
  than a silent "list everything". The rows are `OrderSummary` — the header without its lines,
  which is what a list shows.
- `OrderRepository.List` / `OrderService.Orders`, plus `procurement.ParseStatus`.
- `frontend`: *Beschaffung — Bestellungen* (`/procurement/bestellungen`) — lists the orders,
  names the statuses in words, and approves a draft (the step that commits budget). A failed
  approval keeps the list and names the order; a failed load does not pretend to be an empty
  list. Karma spec, Playwright run with axe, mutation gate.

### Notes
- The orders screen filters what it loaded: the generated client emits path parameters only, so
  no route's `?query=` filter is reachable through `CampusErpApi` yet (docs/15). Lists that need
  paging need that first.

## [0.75.0] - 2026-11-03

The budget screen, and the read route behind it.

### Added
- `GET /api/budget/positions` (role `finanzen`): every position with its balance and
  `available_cents` — the derived figure the Mittelüberwachung applies, stated by the API so a
  client does not re-implement the rule.
- `frontend`: *Budget — Mittelübersicht* (`/budget/haushaltsstellen`) — the second screen and
  the first read screen. Loading, empty and failed states are distinct (a failed load no
  longer also claims the budget is empty). Karma spec, Playwright run with axe, and the
  frontend mutation gate all cover it.

### Fixed
- The OpenAPI generator resolved only *unqualified* field types: a field typed `budget.Position`
  fell through to `string`, so `BudgetPositionResponse.position`/`balance` reached the client as
  strings instead of objects. A qualified name is now resolved in the package it names.

## [0.74.0] - 2026-11-03

Reports: a shared catalogue, and the budget's Mittelübersicht.

### Added
- `internal/platform/reports` — the catalogue of reports this system renders (definition plus
  collector). The worker renders through it and the API lists it, so the reports a client is
  offered and the reports that actually render cannot drift.
- `budget.PositionOverview` (id `budget-position-overview`, "Mittelübersicht"): every position
  with Ansatz, Verpflichtet, Ist and Verfügbar, plus a totals row. The available column is
  derived per position from the balance the guard uses.
- `GET /api/reports` (role `kasse`) returns those definitions, columns included.
- `PositionRepository.ListPositions` and `PositionService.Positions`.

### Changed
- The `report-render` worker kind no longer knows one hardcoded report: it asks the catalogue.
  An unknown id, or a collector whose data source was not wired, is refused rather than
  producing an empty report that looks real.
- `internal/platform/reports` added to `backend/mutation.json`; `core/report` added to the
  OpenAPI generator's package list so `report.Definition` is a typed schema instead of a
  free-form object.

## [0.73.0] - 2026-11-03

The e-invoice generator enforces the standard's code lists, and the catalogue is seeded from
the same lists.

### Fixed
- `core.Validate` accepted **any** non-blank VAT category and payment means code, so a document
  with a category or payment means EN 16931 does not define was generated (or imported) happily.
  It now rejects them (`ErrVATCategoryUnknown`, `ErrPaymentMeansUnknown`) against the code lists
  in the package: UNTDID 5305 (S, Z, E, AE, K, G, O, L, M) and UNTDID 4461 (1, 10, 20, 30, 42,
  48, 49, 57, 58, 59, 97).

### Changed
- The `vat_category` and `payment_means` catalogues are **generated from** those code lists
  (`VATCategories()`, `PaymentMeansCodes()`) instead of being hand-kept in the seed, so the
  catalogue an operator sees and the codes the generator accepts cannot drift. A test compares
  the seeded rows with the lists in both code and label. (docs/11 claimed the generator had
  inline enums to point at the catalogue; it did not — the gap was closed the other way round.)
- The seeded catalogue gains the codes EN 16931 allows that it lacked (VAT L/M; payment means
  1, 20, 42, 49, 57, 97).

## [0.72.0] - 2026-11-03

Two-way matching: an inbound e-invoice is matched to its purchase order and the commitment
becomes an actual.

### Added
- `procurement.OrderService.MatchReceipt(orderNumber, grossCents, reference, by)` — finds the
  order by number, requires it approved and its gross to equal the invoice's, then releases the
  commitment and posts the actual in one step (so the conversion does not double-count), and
  marks the order `received`.
- `OrderRepository.GetByNumber`, and `einvoice.InvoiceService.Match`, which keys on the buyer
  reference (BT-10) the document carries and audits `einvoice.match`.
- `POST /api/e-invoices/{id}/match` (role `finanzen`). A mismatch is `422`, a wrong order state
  `409`, an unknown order `404`.

### Notes
- The match is guarded by the order's state and the amount, not by the invoice's direction:
  nothing marks a stored invoice as inbound or outbound yet (docs/09).

## [0.71.0] - 2026-11-03

CI: the Go module cache actually restores, and a proxy blip no longer reddens a shard.

### Fixed
- `actions/setup-go` looked for the dependency file at the repository root, but the module
  files live under `backend/`: the cache restore failed every run ("Dependencies file is not
  found") and each of the ~25 mutation shards re-downloaded the whole module set. Pointed at
  `backend/go.sum`, so the cache key matches and the set is fetched once per run.
- A transient `proxy.golang.org` failure (`stream error ... INTERNAL_ERROR; received from
  peer`) failed a shard whose package was nowhere near the change — it happened three times.
  The module download and the gremlins install now retry before failing.

## [0.70.0] - 2026-11-02

Approving a payment order binds budget — and cannot exceed it.

### Added
- `kasse.OrderService.Approve` posts a `commitment` for the order's amount on the budget
  position it names (`ANORD:<reference>`), so an approved order has Mittel bound to it
  (docs/10). The binding is posted **before** the status change: a refused approval leaves the
  order awaiting a decision rather than approved without budget.
- The budget module's guard applies, so an order that would exceed its position's available
  Mittel is refused with `ErrBudgetExceeded` (mapped to `422`), not approved.
- `kasse.BudgetPosition.AllocatedCents`: the position is established with its Mittel, and the
  order body's `budget_position` takes `allocated_cents`. It is written when the position is
  first created; naming an existing position does not rewrite its Ansatz.

### Notes
- This exposed a dead end: an order always created its own position with zero Mittel, so
  approval could never have bound anything. The allocation on the position is what makes the
  binding meaningful — and the follow-up is referencing an existing budget position rather than
  creating one inline.
- `kasse.NewOrderService` now takes a `BudgetCommitter` (the budget `PositionService`), wired in
  `cmd/api`; a service wired without one refuses an approval (`ErrNoBudgetCommitter`) rather
  than approving money against no Mittel.

### Tests
- Unit: the approval binds the amount under the order's reference; no committer and an
  over-budget binding both refuse and record no decision.
- Integration: the binding is visible in the position's balance (committed/available), and a
  second order that would exceed what is left is refused with `ErrBudgetExceeded` and stays
  submitted.

## [0.69.0] - 2026-11-02

Every job kind is now reachable from a route.

### Added
- `POST /api/bank-statements/imports` (role `kasse`): queues a bank-statement import
  (`{account_id, content}` -> `202`), the asynchronous counterpart of `POST /api/bank-statements`.
  The bytes travel with the job, so a retry re-imports the same file.
- `POST /api/payment-runs/{id}/exports` (role `kasse`): queues the delivery of a payment run's
  already-rendered SEPA file to the bank channel.
- `jobs.EnqueueSepaExport` — the one enqueue seam docs/17 listed that did not exist (the doc
  was wrong; the code now matches it). `jobs.Exports` exposes it and the statement import.

### Changed
- With all six kinds reachable, docs/17 §7 lists the routes instead of saying which seams are
  still to be wired.

### Tests
- The two new routes in the shared export-route table (args passed through, refusals as
  `account_required` / `content_required` / `invalid_id`, queue failure, no queue configured).

## [0.68.0] - 2026-11-02

Documentation: three claims the code had outgrown.

### Fixed
- docs/23 (and docs/17) said the ledger export "drops it into `ERP_LEDGER_OUTBOX_DIR`". There
  is no such variable: the worker stores the file through the **artefact store**
  (`ERP_OUTBOX_DIR`; `ERP_S3_ENDPOINT` selects an S3 bucket) under
  `ledger/ledger-postings-<timestamp>.csv` (ADR-0011, docs/26), so a directory and an S3
  bucket behave alike.
- docs/23 listed the tie-back reconciliation as not done. It runs on every export: the service
  refuses an export that does not balance against its bookings, and the audit detail records
  `matched`/`missing`/`extra`/`balanced`.
- docs/21 still said "no collection run yet" (docs/20 and docs/22 document one).

## [0.67.0] - 2026-11-02

The ETL run history is queryable.

### Added
- `GET /api/etl/runs?dataset=…&limit=…` (role `admin`): the recent runs, newest first, as
  `{ "runs": [{ "at", "actor", "summary" }] }`. An empty `dataset` matches every dataset. The
  runs are the audit stream read back, so there is no second store (docs/19).
- `audit.Store.Events(ctx, action, subject, limit)`: the read side of the stream `Append`
  writes — newest first, optionable by subject, with the limit clamped (default 20, max 200)
  so a read cannot pull the whole stream by accident.

### Fixed
- The `etl.run` audit event carried **no timestamp** (`At` was never set), so a run's
  recorded time was the year zero. It records the run's own time now, which is what the
  history reports.

### Tests
- The store read against a real database (order, subject filter, limit, an action nobody
  wrote), the limit clamp as a table test, and the handler (dataset and limit reach the seam,
  a non-positive or non-numeric limit is refused as `invalid_limit`, a store failure is 500).

## [0.66.0] - 2026-11-02

The error envelope is the one the contract documents.

### Changed
- `writeProblem` now writes the documented `Problem` shape — `{error, code, request_id}` —
  instead of only `{error}`. The contract had reserved `code` and `request_id` since the
  transport slice and the server never sent them, so a client reading the spec got fields
  that were never there.
- `code` is a stable machine code derived from the status (`not_found`, `unprocessable`, …).
  `writeProblemCode` passes an explicit code where a client should branch on something the
  status cannot express: `invalid_artefact` / `artefact_not_found` on the artefact route, and
  `state_required` / `report_id_required` / `invalid_period` / `export_queue_unconfigured` on
  the export routes.
- `request_id` is the same correlation id the response carries in `X-Request-ID`; it rides on
  the response writer the middleware already installs, so no handler changed its signature.

### Tests
- The envelope through the middleware (code, message, request_id equal to the header), the
  explicit-code path written with no wrapper, every entry of the status→code mapping, and the
  codes pinned on the artefact and export refusals.

## [0.65.0] - 2026-11-02

The remaining export/render kinds are queueable from the API too.

### Added
- `POST /api/ledger/exports` (role `kasse`): `{from, to}` -> `202 {job_id}`, queuing the
  `ledger-export` kind (docs/23).
- `POST /api/reports/renders` (role `kasse`): `{report_id, from, to}` -> `202 {job_id}`,
  queuing the `report-render` kind (docs/25).
- `jobs.Exports` gains `EnqueueLedgerExport` and `EnqueueReportRender`, so every queueable
  export goes through one seam.

### Changed
- The three export routes share the request handling (`decodeExportRequest`, `exportPeriod`,
  `enqueueExport`) and one `{job_id}` response type, so a third route added no second copy of
  the validation rules.

### Tests
- The routes as one table (statistics, ledger, report), every refusal, a queue failure, and
  the no-queue case.

## [0.64.0] - 2026-11-02

The API can queue a statistics export — the first export kind reachable from a client.

### Added
- `POST /api/statistics/exports` (role `kasse`): body `{state, from, to}`, answers `202`
  with the job id. The work runs on the worker role; the file lands in the artefact store and
  is then reachable through `GET /api/artefacts/statistics/<name>`.
- `jobs.NewEnqueuer` / `jobs.Exports`: an **insert-only** River client (no queues, no workers)
  and the plain-argument seam the HTTP layer hands work through, so `httpapi` does not import
  River. `cmd/api` builds it per tenant and applies River's schema before inserting.
- `jobs.PeriodLayout` is exported (was the package-private `ledgerDate`), so the API validates
  a period with the same layout the job carries.

### Changed
- The route is validated at the edge (state required, dates parsed) and answers `503` when no
  export queue is configured, rather than accepting a request it cannot fulfil.
- Docs: docs/27 gains the API-trigger section, docs/17 §7 says which enqueue seams are wired,
  and two stale claims are corrected (docs/16's "auth is still X-Actor", docs/15's route counts).

### Tests
- The route: the happy path (args passed through, `202` + job id), every refusal, a queue
  failure, and the no-queue case. Plus an integration test that proves the whole hand-over —
  an insert-only client queues, a separately-started worker client works the job.

## [0.63.0] - 2026-11-02

Audit events now record the tenant, and two stale doc claims are corrected.

### Changed
- The audit `actor` is qualified with the resolved tenant (`uni-a/alice`), so an audit row
  says which tenant it belongs to. The tenant is already in the request context from the
  tenancy front (ADR-0002); `actor()` now prefixes it. A server with no tenant in context (a
  handler test) is unchanged, so this is a single chokepoint, not a schema change.
- docs/17 lists all six enqueue seams and states plainly that only `EnqueueEtl` is wired to a
  route today. open-questions Q9 no longer calls the repository private.

### Tests
- The actor rules: header fallback, the token subject winning over the header, the tenant
  qualification, and an empty tenant not qualifying; plus a guard through the tenancy front
  that the resolved tenant is the one the actor carries.

## [0.62.0] - 2026-11-02

Statistics exports: the per-state layout is data, and the last job kind is wired.

### Added
- `internal/core/statistics`: the per-state statistics export (IR-4 / FR-P9, docs/27). A
  `Format` names a state's columns, their order, their headings and the field separator, and
  the `Registry` holds one per state — so a state is added or changed by editing *data*, never
  the pipeline. Only the set of canonical fields is code. `Group` aggregates neutral entries
  into one record per key; `Render` projects records through a format. Formats are validated on
  construction, so a malformed layout fails loudly instead of writing a wrong file.
- `statistics-export` job kind (`internal/platform/jobs`): args `{State, From, To, Actor}`; the
  worker validates its preconditions and hands the I/O to the role, like the other export kinds.
- The worker role groups the period's confirmed bookings by purpose, renders them with the
  state's format and stores the file at `statistics/<state>-<from>-<to>.csv`;
  `GET /api/artefacts/statistics/<name>` serves it (role `kasse`).
- ADR-0012 (per-state statistics formats as data) and docs/27.

### Changed
- The `statistics` interface (IR-4) is `implemented` in the registry. Its layouts are
  **project-defined placeholders** (`default`, `bw`, `by`), marked as such — the offices' real
  layouts are not published (open-questions Q8) and replace a placeholder without a code change.
- `jobs.safeName` became `jobs.plainFileName` and its messages no longer name SEPA: both export
  kinds share the outbox rule, so the rule and its wording now live in one place.

### Tests
- The statistics format rules (validation, projection, aggregation, rendering) as table tests,
  the job kind's args and every refusal, and a worker integration test that runs a
  `statistics-export` job through the real River runtime and asserts the seam receives its args.

## [0.61.0] - 2026-11-02

Artefacts: reports, ledger and SEPA exports are fetchable too.

### Added
- `GET /api/artefacts/{kind}/{name}` (role `kasse`): serves stored artefacts of kind
  `dunning`, `reports`, `ledger` or `sepa`. Two path segments rather than a wildcard, so the
  router and the contract generator handle it unchanged - and the kind allowlist keeps the
  route from becoming a read-anything primitive over the store. The kind fixes the media
  type, the extension overrides it (a CSV report is `text/csv`), anything else is
  `application/octet-stream`. An unknown kind is 400, an absent artefact 404.
- The contract generator learned an `artefact` response kind documenting the three media
  types the route can return.
- Tests: the helper rules (allowlist, bare names, traversal refused, media types) as a table
  test, and the route over HTTP for all four kinds plus the refusal and not-found cases.

## [0.60.0] - 2026-11-02

Documents are fetchable: GET a dunning notice's stored PDF.

### Added
- `GET /api/dunning/notices/{id}/document` (role `kasse`): serves the notice's stored PDF
  from the artefact store, so it works over a directory or an S3 bucket alike. No document
  recorded is 404, a malformed id 400, and a recorded key whose artefact cannot be read 500
  - a server-side inconsistency, not a client error.
- `httpapi.Deps.Artefacts` (`storage.Store`) wires the store into the server.
- The contract generator learned a `pdf` response kind, so the route documents an
  `application/pdf` binary response instead of the nearest fit.
- Tests: the route end to end against the real database and a real directory store -
  served bytes with the right content type, no document, malformed id, unknown id. The
  harness gained a shared artefacts directory (memoised: `t.TempDir()` returns a new
  directory per call, which had the server and the test looking in different places).

## [0.59.0] - 2026-11-02

Storage: RustFS is the S3 endpoint, and the S3 path is verified against it.

### Changed
- The local stack's S3 service is **RustFS** (`rustfs/rustfs`), still behind the `s3`
  profile, replacing an endpoint whose vendor is now commercial. Ports and credentials are
  unchanged in shape (`S3_PORT`, `S3_CONSOLE_PORT`, `RUSTFS_ACCESS_KEY`,
  `RUSTFS_SECRET_KEY`).
- **The S3 store is verified against a real server, not only the synthetic one:**
  `storage.TestS3RoundTrip` passes against RustFS (bucket creation, Save/Get with nested
  keys, a missing key failing, an escaping key refused). `docs/26` and ADR-0011 say so, and
  `docs/build-and-deploy.md` shows how to start it.
- The Go client is still minio-go (a library, not the product); the endpoint is deliberately
  not tied to one vendor.

## [0.58.0] - 2026-11-02

Dunning: read a notice back, with its document key.

### Added
- `kasse.DunningRepository.GetNotice` / `DunningRepo.GetNotice`: load a notice by id,
  including the key of its stored document; a missing id is `ErrNotFound` in the repo's
  idiom, alongside the other read methods.
- `DunningService.Notice`: the read path the document route and any consumer need. The
  write half (rendering and storing the notice's PDF) shipped in 0.52.0.
- Tests: the service read path and its not-found propagation, plus a real-database round
  trip (insert, read with no document yet, record the key, read it back, missing id, and
  recording a document on a missing notice failing).

## [0.57.5] - 2026-11-02

Docs: no pointers to the vendor product or its delivery archives.

### Removed
- `docs/05-productive-source-inventory.md`: it existed only to inventory the vendor's
  delivery archives, so nothing of it could be kept without naming them.

### Fixed
- `docs/08` and `docs/14` no longer name the delivery archives or the directory they were
  handed over in, and the `unzip` lines that pointed at them are gone; the analysis they
  carry is unchanged. `docs/open-questions.md` points at `docs/08` instead of the removed
  file. Verified: no tracked file names the vendor product, its archives or that directory.

## [0.57.4] - 2026-11-02

Gate: the mutation sweep contains its own scratch space.

### Fixed
- `tools/run_tests.sh` now points `TMPDIR` into the same per-package cache dir it already
  cleans, so the object files and coverage profiles gremlins and the Go toolchain spill into
  `/tmp` are removed with it. A sweep leaves no more space behind than it found: previously
  8 GB of leftover scratch filled a 16 GB `/tmp` and made the back half of a sweep report
  `below floor` on packages that were green.

## [0.57.3] - 2026-11-02

Gate: a failing mutation run says why.

### Fixed
- `tools/run_tests.sh` reported `below floor` for any non-zero gremlins exit, including a
  build error, a coverage-gathering failure or a full disk. It now prints the reason
  (`Killed:`/`Test efficacy`/`failed to`/`fatal`/`quota`) under the failure, so a mechanical
  failure cannot masquerade as a regression - which is exactly how a full `/tmp` sent me
  hunting eight packages that were green.

## [0.57.2] - 2026-11-02

Local stack: MinIO is opt-in.

### Changed
- `docker-compose.yml` puts `minio` behind the `s3` profile, so `docker compose up` no
  longer tries to pull an image the registry may refuse; the default artefact store is a
  directory (`docs/26`) and needs nothing. Start it with `docker compose --profile s3 up
  minio` — `docs/build-and-deploy.md` says how, with the `ERP_S3_*` variables.

## [0.57.1] - 2026-11-02

Docs: bring the indexes up to date.

### Fixed
- The ADR index stopped at 0008; it now lists 0009 (documents via a service), 0010 (a
  report is a definition plus rows) and 0011 (artefacts go through one keyed store).
- The README's document map stopped before the platform docs; it now points at docs/17
  (platform), 19 (ETL), 23 (ledger postings), 24 (documents), 25 (reports) and 26 (storage),
  and the ADR row now reads 0001-0011.

## [0.57.0] - 2026-11-02

Worker: the report-render job kind (closes the code work on #38).

### Added
- `jobs.KindReportRender` with `ReportRenderArgs{ReportID, From, To, Actor}` and
  `ReportRenderWorker`: builds a report by id over a period through `ReportRenderFunc`,
  refuses a missing id, a malformed period and a render that stored nothing, and returns
  failures so River retries. `EnqueueReportRender` is the API-side seam.
- Registered in `StandardWorkers`; the worker role knows `kasse-daily-close`, collects the
  period's bookings, renders the report and stores the PDF through the document service and
  the artefact store.
- Tests: the kind, the happy path, every refusal, the logger. Docs: `docs/17` §7 and
  `docs/25`.

## [0.56.0] - 2026-11-02

Storage: the S3 path is verified.

### Changed
- `storage.TestS3RoundTrip` now serves an in-process S3-compatible server (gofakes3) when
  `ERP_S3_TEST_ENDPOINT` is unset, so the S3 store is exercised on every build instead of
  being written-but-unproven: bucket creation (idempotent), Save/Get round trips with nested
  keys, a missing key failing, and an escaping key refused — all through the minio-go client.
  `ERP_S3_TEST_ENDPOINT` still points the same test at a real endpoint.
- MinIO's own image is refused by this environment's registry, so verification runs against
  an S3-compatible server rather than MinIO itself. ADR-0011 and `docs/26` updated.

## [0.55.0] - 2026-11-02

Storage: one keyed artefact store (ADR-0011).

### Added
- `internal/platform/storage`: `Store` (`Save`/`Get`), a directory implementation and an
  S3-compatible one (minio-go), plus `FromEnv` (`ERP_S3_ENDPOINT` → S3, else a directory)
  and `CheckKey` (empty/absolute/escaping keys refused). Mutation target.
- The three artefact writers now share it: dunning notices (`cmd/api`), and the ledger and
  SEPA exports (`cmd/worker`), with keys `dunning/<id>.pdf`, `ledger/…csv`, `sepa/…xml`.
  `documents.Store` is the one-method subset, so no adapter is needed.
- **ADR-0011** and `docs/26-storage.md`, which states plainly that the S3 path is
  written but not yet verified end to end (the MinIO image could not be pulled here).

## [0.54.0] - 2026-11-02

Reports: the model and the first report (part of #38).

### Added
- `internal/core/report`: a report is a `Definition` (id, title, ordered columns with key,
  heading and alignment) plus rows of strings. `Validate` refuses an incomplete definition,
  duplicate keys and rows that do not match the columns; `MarshalCSV` and `MarshalHTML`
  render it, with everything escaped by `html/template`. Mutation target.
- `kasse.DailyCloseReport` + `DailyCloseDefinition`: the cash book's daily-close report —
  one row per booking and a closing row with income, expense and the net.
- **ADR-0010** (a report is a definition plus rows, and why not a template per report) and
  `docs/25-reports.md`.

## [0.53.0] - 2026-11-02

Worker: the sepa-export job kind (part of #38).

### Added
- `jobs.KindSepaExport` with `SepaExportArgs{RunID, Actor}` and `SepaExportWorker`:
  delivers a payment run's already-rendered pain.001 file to the bank channel through
  `SepaExportFunc`. Refuses a missing run, an empty file name or content, and a name
  that is nested or escapes the outbox; failures are returned so River retries.
- Registered in `StandardWorkers`; the worker role fetches the run file from the
  cash-book payment-run repository and writes it to `ERP_SEPA_OUTBOX_DIR`.
- Tests: the kind, the happy path, every refusal, the logger.

## [0.52.0] - 2026-11-02

Dunning notices get their document (increment 2 of #39).

### Added
- `documents.Store` and `documents.DirStore`: keep rendered documents under a key,
  creating parents; a key that is empty, absolute or escapes the root is refused.
- `DunningNotice.DocumentKey` and `kasse.DunningRepository.SetNoticeDocument`
  (migration 0019): the notice row carries the reference to its stored artefact.
- `kasse.DunningDocuments` + `DunningService.WithDocuments`: issuing a notice renders
  and stores its PDF and records the key. A render or store failure fails the issue, so
  a notice never claims a document it does not have; the audit detail names the key.
- The API role builds the issuer from `ERP_DOCUMENTS_URL` and `ERP_DOCUMENTS_DIR`
  (`ERP_ORGANISATION` for the issuer line); with either unset, notices are issued
  without a document instead of failing.
- Tests: the store (save, nesting, every refusal), the hook (key recorded, audit names
  it, no documents -> no key, render failure, record failure).

## [0.51.0] - 2026-11-02

Ledger export: the worker job kind and the hand-over seam (increment 3 of #42).

### Added
- `jobs.KindLedgerExport` with `LedgerExportArgs{From, To, Actor}` and
  `LedgerExportWorker`: renders a period's export and hands it over via
  `LedgerExportFunc`; refuses a missing actor or a malformed period and returns a
  failure so River retries it. `EnqueueLedgerExport` is the API-side entry point.
- `LedgerExportService.Export` now enforces the tie-back invariant: an export that
  does not balance against its bookings is refused (`ErrLedgerUnbalanced`) and never
  handed over.
- The worker role builds the exporter from the cash-book ledger service and writes the
  file to `ERP_LEDGER_OUTBOX_DIR` (the seam object storage or SFTP replaces).
- Registered in `StandardWorkers`; tests for the kind, the happy path, every refusal,
  the logger and the invariant.

## [0.50.0] - 2026-11-02

Ledger export: tie-back reconciliation (increment 3 of #42, first half).

### Added
- `ledger.Reconcile`: compares the booking references of a period with the postings
  rendered from them, counts each side (so a duplicate cannot hide behind a matching
  reference) and reports the surplus as `Missing`/`Extra` with `Balanced()`.
- `LedgerExportService.Export` now reconciles the run and records it on the audit
  stream (`matched`, `missing`, `extra`, `balanced`), so an export that does not
  account for its bookings is visible rather than silent.

## [0.49.0] - 2026-11-02

Worker: the statement-import job kind (part of #38).

### Added
- `jobs.KindStatementImport` with `StatementImportArgs` and `StatementImportWorker`:
  imports one bank-statement file (content travels with the job, so a retry re-imports
  the same bytes), refuses a missing account or empty content, and returns failures so
  River retries them. `EnqueueStatementImport` is the API-side entry point.
- `StatementImportFunc` keeps the jobs package free of a dependency on the module that
  owns the import, exactly as `DatasetRunner` does for ETL.
- Registered in `StandardWorkers`; the worker role builds the importer from the cash-book
  statement service. Tests for the args kind, the happy path, every refusal and the
  logger.

## [0.48.0] - 2026-11-02

Documents: HTML notice + PDF service (part of #39).

### Added
- `internal/platform/documents`: `DunningNotice.HTML` (deterministic, escaped HTML from
  a dunning case; golden-tested) and `Renderer` / `NewHTTPRenderer`, which posts the
  HTML to the document service and returns the PDF. Mutation target.
- One new deploy component: the document service (stateless HTML→PDF), gated in the
  chart (`pdf.enabled`, off by default) with values in `values.yaml`.
- **ADR-0009** (render documents with a service, and why not an in-process PDF
  library), `docs/24-documents.md`, and a short target-container overview with the
  document path in `docs/c4/02-container.md`.

## [0.47.0] - 2026-11-02

Ledger posting export: collection + service (increment 2 of #42).

### Added
- `kasse.LedgerRepo.ListBookingsForExport`: every booking made on a day inside the
  period, in (business date, booking id) order. The reference is the import reference
  when the booking has one and a derived `BK-######` otherwise - it is the export's
  idempotency key.
- `kasse.LedgerExportService.Export`: maps each booking to a posting (income credits
  the counterpart account, expense debits it), renders the `cash-postings/v1` export,
  and records the run on the audit stream. It refuses a blank actor, incomplete
  accounts, an inverted period and an empty period rather than emitting an empty file.
- `kasse.LedgerAccounts` (+ `DefaultLedgerAccounts`) for the cash/income/expense
  account mapping, validated before use.
- Tests: the posting mapping, ordering and determinism, the full error table
  (actor, accounts, period, empty period, repository, unknown kind, audit), and an
  integration test that seeds a day with two bookings and exports them twice -
  byte-identical - plus the empty-period refusal.

## [0.46.0] - 2026-11-02

Ledger posting export, format + renderer (increment 1 of #42).

### Added
- `internal/core/ledger`: the `cash-postings/v1` export that carries confirmed cash
  bookings to the ledger side. `Marshal` is versioned, validated, sorted by booking
  date then reference and field-trimmed, so the same booking set always renders
  byte-identically; references are the idempotency key. Mutation target (100 %).
- `docs/23-ledger-postings.md`: the format and how it is consumed.

## [0.45.0] - 2026-11-02

Make the repository vendor-neutral.

The project is an independent Go rebuild; the legacy implementation and its data are
only a reference. Nothing in the repository may point at a specific commercial
product, so every vendor identifier was removed (~600 occurrences across ~60 files):

- Product/company names (the campus-management and legacy-ERP brands, the vendor's
  name and domain) replaced with neutral wording: "the legacy system", "the reference
  implementation", "the vendor".
- Vendor module product codes replaced by their domain ("the cash-book module",
  "the HR module", ...), and vendor package/file paths replaced by role descriptions
  ("the reference parser", "the reference application", "the SOAP service catalogue").
- The interface registry's vendor entries are now keyed generically (`erp-soap`,
  `card-terminal`) with neutral names, and the contract/registry golden were
  regenerated.
- The module catalogue and the source inventory no longer cite vendor marketing or
  file inventories; they describe the domains to be rebuilt.

Verified: no vendor token remains anywhere in the tracked tree; policy_check green;
`make test` green (4 suites, fail=0); contract regenerated.

## [0.44.0] - 2026-11-02

Prepare the repository for public release.

### Removed
- `schema/` - a `pg_dump --schema-only` of the proprietary third-party database plus
  its derived TSVs (~7.6 MB). It is generated from that dump by `scripts/analyze.sh`,
  so it is now git-ignored and reproducible locally instead of committed. The docs
  that quoted measurements from it still do, and now say how to regenerate it.

### Added
- `LICENSE` (Apache-2.0) and `CONTRIBUTING.md`.
- README: a "what is not in this repository" section (the vendor dump, the generated
  artefacts, the vendor XSDs/Java/JARs), a trademark note (the reference implementation/the legacy university ERP are
  the vendor products; this is an independent reimplementation), and an up-to-date status
  paragraph (the old one still said CAMT.053 was unimplemented).

### Fixed
- `tools/gen_erm.py` / `tools/test_erm.py`: the legacy ER models are generated from
  `schema/`, so the generator and its self-test now degrade gracefully when that
  directory is absent (target model only, legacy checks skipped) instead of failing.

### Changed
- `.github/workflows/rerun-flaky.yml` renamed to `rerun-on-runner-shutdown.yml` to
  match what the changelog and the workflow's own name say.

## [0.43.0] - 2026-11-02

CI: retry runs killed by a runner preemption, and only those (#41).

### Added
- `.github/workflows/rerun-on-runner-shutdown.yml` — on `quality` completion, if a run
  failed and **every** failed job's log carries the runner-shutdown marker
  (`received a shutdown signal`, exit 143), it re-runs the failed jobs. It declines
  when any failure looks real, when there are no failed jobs, and when the run has
  already been re-run once (`run_attempt > 1`), so it cannot loop. Also runnable via
  `workflow_dispatch` with a `run_id` and a `dry_run` flag for inspection.

## [0.42.0] - 2026-11-01

XSD validation wired up, and the bugs it found (#40).

Wiring `golden.ValidateXSD` into the SEPA and CAMT tests (schema path from
`ERP_XSD_PAIN001/Pain008/CAMT052/053/054`, skipped when unset) exposed real defects:

### Fixed
- **CAMT parsers read the counterparty from a non-ISO path.** `Dbtr/Nm` and
  `Dbtr/Acct/Id/IBAN` do not exist in ISO 20022 — a party is a `Party40Choice`
  (`Dbtr/Pty/Nm`) and the account is a sibling (`DbtrAcct/Id/IBAN`). On a real bank
  file the counterparty name and IBAN were therefore empty. Fixed in camt.052/053/054.
- **pain.008 emitted an invalid creditor scheme name** (`SchmeNm/Cd`); the GBIC
  profile requires `Prtry`.
- **pain.008 made `DbtrAgt` optional**, but it is 1..1: a transaction without a
  debtor BIC now fails with `ErrDebtorAgentRequired` instead of producing a file the
  bank rejects.

### Changed
- The CAMT test fixtures are now schema-valid (they were hand-written and had never
  been validated): `Sts`, `BkTxCd`, and the ISO party/account nesting.

## [0.41.0] - 2026-11-01

Platform: the worker runtime (River) — the last Phase-0 gap (#44).

### Added
- **`internal/platform/jobs`** — the async runtime on River/Postgres (ADR-0007):
  `Migrate` (idempotent River schema), `New` (client with the standard workers),
  `StandardWorkers`, and the `etl` job kind (`EtlArgs{Dataset, Actor}`) which runs a
  registered dataset and puts the run on the audit stream. `EnqueueEtl` is the
  enqueue seam for the API role.
- `cmd/worker` is wired: opens the pool, migrates, builds the ETL registry (legacy
  source pool when `ERP_LEGACY_DATABASE_URL` is set), starts the client, blocks until
  SIGINT/SIGTERM and drains with a bounded stop.
- Tests: the `etl` kind's args/error paths, and an integration test that migrates
  River's schema, starts a client, enqueues a job and waits for it to complete.

### Changed
- The module moves to **Go 1.26** (River v0.51 requires it): `go.mod` and the CI
  workflows' `setup-go`. The local docker fallback image follows (`golang:1.26-alpine`).
- `internal/platform/jobs` is a mutation target (100 % efficacy).

## [0.40.0] - 2026-11-01

Bank statement import: CAMT.052 (intraday account report) (#34).

### Added
- **`internal/core/camt052`** — parses an ISO 20022 camt.052 bank-to-customer
  account report into the shared `bankstmt` shape; `Bal` `OPBD`/`CLBD` are mapped
  when present and left zero otherwise. Mutation target added.
- `StatementService.Import` routes the XML container: `BkToCstmrAcctRpt` →
  CAMT.052, `BkToCstmrDbtCdtNtfctn` → CAMT.054, otherwise CAMT.053.
- Tests: a golden report (balances + credits + debits), the no-balance case, error
  cases, amount/date parsing, and an integration test reconciling a CAMT.052 line
  against a paid order.
- `bank-camt052` added to the interface registry (IR-2); the statement-import doc and
  the C4 external-systems diagram note the format.

## [0.39.0] - 2026-11-01

Tooling: split the quality gate into a fast suite and the mutation gates (#32).

### Changed
- `tools/run_tests.sh` takes a `STEPS` selector: `fast` (Go vet/build/unit+integration,
  Angular build + unit, the ER self-test, the API-contract drift guard) and
  `mutation` (gremlins per package + Stryker); default is both.
- **`make test`** now runs the fast suite (seconds when warm — it previously ran
  everything and took ~13 min, which is over an interactive command budget);
  **`make mutation`** runs the mutation gates alone; **`make test-all`** runs both.
  `make quality` runs policy + contract + the full suite. CI is unaffected (it
  already splits these into separate jobs).
- README / AGENTS / `docs/build-and-deploy.md` updated for the split.

## [0.38.0] - 2026-10-28

ETL: run datasets from the API (#26).

### Added
- **`etl.Registry`** — binds a `Runner` to named datasets (`Names`, `Run`,
  `ErrUnknownDataset`), so a run no longer needs hand-wiring.
- **API** (contract-first): `GET /api/etl/datasets` and `POST /api/etl/runs`
  `{"dataset":"..."}` (returns the `etl.Report`), behind `erp-admin`.
  `contracts/openapi.yaml` + generated client regenerated (58 operations,
  65 schemas); drift guard green.
- `ERP_LEGACY_DATABASE_URL` — the ETL source pool (legacy DSN); when unset the
  canonical pool is used (dev / seed). `cmd/api` opens the second pool and builds
  the registry from `lookup.StandardDatasets`.
- Tests: registry (names sorted, blank/duplicate handling, delegation, unknown
  name), the HTTP handlers, and the status mapping.

## [0.37.0] - 2026-10-28

Bank statement import: CAMT.054 (#23).

### Added
- **`internal/core/camt054`** — parses an ISO 20022 camt.054 debit/credit
  notification into the shared `bankstmt` shape. A notification carries no
  balance, so the parsed statements hold lines only. Mutation target added.
- `StatementService.Import` detects the XML container: `BkToCstmrDbtCdtNtfctn`
  → CAMT.054, otherwise CAMT.053; both reconcile identically (matched / returned /
  unmatched, idempotent on the file hash).
- Tests: a golden notification (credits + debits, counterparty, purpose, dates),
  the no-balance invariant, error cases, amount/date parsing, and an integration
  test that imports a CAMT.054 and reconciles a line against a paid order.
- `bank-camt054` added to the interface registry (IR-2); `docs/legacy/kbs-statement-import.md` notes the format.

## [0.36.0] - 2026-10-28

SEPA direct-debit collection run (#22).

### Added
- **`direct_debit_run`** + **`direct_debit_run_item`** (migration `0018`) — one
  produced pain.008 file per collection date/sequence type, with the stored file,
  its hash, and one row per collected mandate (`UNIQUE (run_id, mandate_reference)`).
- **`kasse.CollectionRun`** domain + repo + audited service `CreateCollectionRun`:
  resolves each item's mandate, builds a `sepa.DirectDebit`, stores the file, and
  is idempotent on the reference. Fails closed on an unknown/inactive mandate, a
  sequence-type mismatch, a blank reference or a duplicate mandate in one run.
- **API** (contract-first): `POST /api/direct-debit/runs` and
  `GET /api/direct-debit/runs/{reference}/file`, behind `erp-kasse`.
  `contracts/openapi.yaml` and the generated client regenerated (56 operations,
  63 schemas); the drift guard passes.
- Tests: run-level validation (every branch), an integration test (produced file
  bytes + hash, idempotent re-run, unknown mandate, sequence mismatch, the DB-level
  per-run uniqueness, audit), the HTTP handlers and the status mapping.
- `docs/22-collection-runs.md`; the interface registry's `bank-sepa-sdd` reference
  now points at the run.

## [0.35.0] - 2026-10-28

SEPA direct-debit mandates.

### Added
- **`direct_debit_mandate`** (migration `0017`) — the mandate reference, debtor,
  signature date and sequence type, with a constrained sequence type and no hard
  deletes.
- **`kasse.Mandate`** + `NewMandate` validation (reference, ISO 7064 IBAN, ISO 9362
  BIC via `core/sepa`, signature date, sequence type) and the mandate service
  (`mandate.create` audited).
- **API** (contract-first): `POST /api/direct-debit/mandates` and
  `GET /api/direct-debit/mandates/{reference}`, behind `erp-kasse` like dunning.
  `contracts/openapi.yaml` and the generated Angular client regenerated (54
  operations, 60 schemas).
- Tests: domain validation, an integration test (persist + get + duplicate +
  missing + audit), the HTTP handlers, and the status mapping.
- `docs/21-mandates.md`.

## [0.34.0] - 2026-10-28

Interface: SEPA direct debit (pain.008.001.08).

### Added
- **`sepa.DirectDebit`** (`internal/core/sepa/pain008.go`) — generates an ISO 20022
  pain.008.001.08 direct-debit initiation, the collection counterpart of the
  credit transfer: stdlib-only, deterministic, pinned by a golden file. The SEPA
  creditor identifier goes in `CdtrSchmeId`, each transaction carries its mandate
  (`MndtId` + `DtOfSgntr`), and sequence type / local instrument are constrained to
  the SEPA sets. Full validation with sentinel errors (`docs/20`).
- Tests: a golden file, determinism, structure (incl. optional agent blocks and
  empty-currency default), the length-limit boundaries and every validation branch.

### Changed
- The interface registry marks `bank-sepa-sdd` **implemented** (was planned);
  `testdata/interfaces.txt` regenerated.

## [0.33.0] - 2026-10-28

Deployment: ADR-0007 infrastructure in the chart, gated and off by default.

### Added
- **CloudNativePG `Cluster`** (`postgres-cluster.yaml`) and a **PgBouncer `Pooler`**
  (`pgbouncer-pooler.yaml`), one cluster with one database per tenant (ADR-0002).
- **Scheduler `CronJob`** (`scheduler.yaml`) for the nightly batch.
- **Redis `Deployment` + `Service`** (`redis.yaml`) for sessions / rate-limit /
  cache.
- `values.yaml` gains a gated ADR-0007 section (postgres, scheduler, redis,
  objectStorage); each piece can instead point at a managed/on-prem equivalent.
- `docker-compose.yml` gains MinIO and Redis, so local / single-host installs match
  the production shape.

### Changed
- `deploy/README.md` documents the ADR-0007 map (chart-provided vs external); ADR-0007
  records the revision.

## [0.32.0] - 2026-10-28

ETL: the two reference datasets that hold real data (bank codes, insurers).

### Added
- **`lookup.BankDirectoryDataset`** — imports the German bank-code directory
  (`kbs.blz`, **3,732 rows verified against the restored dump**) into the
  `bank_leit_zahl` catalogue namespace: BLZ → name / city / BIC. Useful for SEPA
  and IBAN work.
- **`lookup.InsurersDataset`** — imports the statutory health insurers
  (`sva4.k_krankenkasse`, **1,677 rows**) into the `krankenkasse` namespace.
- **`lookup.StandardDatasets`** — the registry of reference datasets sourced from
  the productive legacy data.
- An integration test driving both datasets against synthetic tables of the same
  measured shape, including re-run idempotency.

Both reuse the existing catalogue pipeline (`LegacyCatalogueSource` +
`CatalogueSink`), so a reference list is rows, not a new table.

## [0.31.0] - 2026-10-28

ETL: the counter book (kbs.schalterbuch) as a cash-book dataset.

### Added
- **`kasse.LegacyCounterBookSource` / `CounterBookSink`** — imports the legacy
  counter book into `cash_book_day` + `cash_booking`, grouped by business day.
  The legacy table is empty, so the mapping is an explicit inference: **amount sign
  → kind** (no booking-type code list exists in `kbs`), purpose from
  `grund1`+`grund2`, and the day created closed with a computed closing balance
  (`docs/legacy/kbs-cashbook.md`).
- **Migration `0016`** — a `legacy_ref` column on `cash_booking` with a
  partial-unique index, giving imported bookings a natural key for idempotent
  upserts without weakening a day's own integrity.
- **`Service.ImportCounterBook`** and the repo's `UpsertImportedDay` /
  `UpsertImportedBooking` / `SetDayClosing` — a validate-all-then-write import that
  goes through the day service (one `cashbook.import` audit event per day), so
  every write stays on the audit chain.
- Unit tests for the sign→kind / amount mapping and the import validation; an
  integration test for reconciliation, per-day closing balance, idempotency and
  fail-closed behaviour.

## [0.30.0] - 2026-10-28

Cash-book legacy findings (measured), and why the dataset is deferred.

### Added
- **`docs/legacy/kbs-cashbook.md`** — the FR-K1 group measured from the restored
  sample dataset: `schalterbuch` / `bu` / `kass` / `geing` / `dr` are all **empty**
  (0 rows), so the mapping is unvalidatable before a production extract (the same
  conclusion as payment orders); the three god tables (152 / 115 / 96 columns), the
  denomination-count table, and the absence of any booking-type code list.

### Changed
- **Correction:** `docs/plan/kbs-pilot.md` grouped `sb`/`sbprot` under the cash
  book; they are the **clerk (Sachbearbeiter) master + change log**, with a
  per-area `tabr_*` rights model. Recorded, with a structural security finding on
  the unsalted-looking `sb.pass_wort`.
- `docs/19` records the cash-book dataset as deferred, with the two settled design
  points (a `legacy_ref` natural key and routing the import through the day
  service).

## [0.29.0] - 2026-10-28

ETL: tax offices (kbs.finanzamt) and foundations (kbs.stift), completing S7.

### Added
- **Tax offices + foundations datasets** — `LegacyTaxOfficeSource` /
  `TaxOfficeSink` and `LegacyFoundationSource` / `FoundationSink` move
  `kbs.finanzamt` and `kbs.stift` into canonical `tax_office` and `foundation`.
  They add the two remaining dataset shapes: an integer year and a carry-over
  flag (docs/19); the unset-flag encoding is marked inferred.
- **`TaxRepository.UpsertTaxOffice` / `UpsertFoundation`** and
  **`TaxService.ImportTaxOffices` / `ImportFoundations`** — idempotent, once-audited
  batch imports (`tax_office.import`, `foundation.import`) that validate the whole
  batch before writing.
- Tests: the flag mapping and an integration test covering both pipelines
  (reconciliation, idempotency, and fail-closed on a blank description).

## [0.28.0] - 2026-10-28

ETL: the VAT catalogue (kbs.mwst_art) as a second dataset.

### Added
- **`kasse.LegacyVATSource` / `kasse.VATSink`** — move the legacy VAT catalogue
  into canonical `vat_type`: percent → integer basis points, the 1-char `U`/`V`
  flag → the enumerated kind, no sign column (docs/19). An unset flag defaults to
  output tax, marked as inferred.
- **`TaxRepository.UpsertVATType`** and **`TaxService.ImportVATTypes`** — an
  idempotent, once-audited batch import (`vat_type.import`) that validates the
  whole batch before writing, so an ETL re-run converges and a bad row fails
  without a partial load.
- Unit tests for the percent/basis-point and flag/kind mapping; an integration
  test covering reconciliation, re-run idempotency, drift, in-place update and
  fail-closed behaviour on synthetic legacy rows.

## [0.27.0] - 2026-10-28

Deployment: wire the platform config into the chart and compose.

### Fixed
- The api's Kubernetes **readiness** probe now targets `/readyz` (which pings the
  database) instead of `/healthz`; liveness stays `/healthz`, so a database blip
  no longer marks a serving process not-ready nor restarts it (NFR-6).
- The chart and `docker-compose.yml` now pass the platform's configuration to the
  api: `ERP_TENANT_DSN` (multi-tenant DSN template, from the db Secret) and
  `ERP_OIDC_ISSUER`/`ERP_OIDC_AUDIENCE`/`ERP_OIDC_JWKS_URL` — without which a
  deployed api could not authenticate or serve more than one tenant (ADR-0002/0003).
- The compose api gains a `/readyz` healthcheck.

## [0.26.0] - 2026-10-28

ETL: the run history is the audit stream, so a run flags source drift.

### Added
- **`audit.Store.LastEvent(action, subject)`** — read the most recent matching
  event, so a caller can look back at what it recorded previously.
- **`etl.AuditLedger` / `Runner.WithLedger`** — a dataset's previous checksum is
  derived from its last `etl.run` audit event (no separate ledger table). A run
  then reports `Previous` and `Drift` when the source has changed since last
  import; `afterText`/`checksumOf` keep the write and parse of the detail in step.

### Changed
- `docs/19` documents the ledger; the integration test asserts no-drift on an
  unchanged re-run and drift after a source change.

## [0.25.0] - 2026-10-28

ETL: the re-runnable, reconcilable legacy → canonical pipeline (roadmap S8).

### Added
- **`internal/platform/etl`** — a `Dataset` (named `Source` → `Sink`), a `Runner`
  whose runs are idempotent, reconcile (`ErrReconcile` on a dropped row), fail
  closed on a bad row (no partial load) and append one `etl.run` audit event. A
  schema-driven `QuerySource` turns a legacy table into records via a SQL query +
  mapper (`docs/19`).
- **First pipeline** — `internal/modules/lookup/etl.go` moves a legacy key table
  into the single canonical catalogue (`LegacyCatalogueSource` + `CatalogueSink`),
  with an integration test covering reconciliation, re-run idempotency, in-place
  update and fail-closed behaviour against synthetic fixtures.

### Changed
- `backend/mutation.json` grades `internal/platform/etl`; the stale
  `internal/modules/` exemption (module placeholders) is removed — the modules
  are graded targets.
- `docs/plan/kbs-pilot.md` records S8 as in progress; the parallel run remains
  blocked on a production extract (ADR-0001).

## [0.24.0] - 2026-10-28

Authorisation: per-route role gates on top of authentication.

### Added
- **Per-route authorisation** — `httpapi.authorize` / `authorizeRead` wrap every
  `/api/*` route with the role that owns it (`erp-kasse`, `erp-finanzen`,
  `erp-klr`, `erp-personal`; reads also admit `erp-reader`; `erp-admin` may call
  anything). The gate fails closed if a route is reached without a principal, and
  is a pass-through only when no verifier is configured. The policy lives next to
  the route, so it cannot drift from the table (`docs/17`).
- `internal/httpapi/authz_test.go` — the authorisation matrix (module role,
  reader, admin, wrong module, no role, open route, missing and bad tokens).

### Changed
- `docs/17` records the role model; role-based authorisation is no longer an
  open item.

## [0.23.0] - 2026-10-26

Platform spine: authentication, tenancy, readiness and the interface registry.

### Added
- **`internal/platform/auth`** — stdlib OIDC: RS256 JWTs verified against a JWKS
  (`JWKVerifier`, `RemoteKeySet`, `StaticKeySet`) with a `Principal` and coarse
  roles. The HTTP layer requires a bearer token on `/api/*` when configured and
  records the subject as the audit actor; `docs/17`.
- **`internal/platform/tenant`** — host-based tenant resolution, a DSN-template
  pool registry (lazy open + migrate, database-per-tenant) and `httpapi.Tenancy`,
  a multi-tenant request front. Wired in `cmd/api` via `ERP_TENANT_DSN`; proven by
  an integration test against two real databases (ADR-0002).
- **`/readyz`** — readiness that pings the database and returns 503 when it is
  down (NFR-6); `/healthz` stays pure liveness.
- **`internal/platform/interfaces`** — a validated registry of the external
  interfaces (bank, e-invoicing, OIDC, LBV, ELSTER, statistics, DMS, the reference implementation
  SOAP, KfW, BSOS terminal), seeded from requirements §5 and `docs/14`, pinned
  by a golden file.
- **`internal/core/golden`** — a golden-file + XSD test harness used by the SEPA
  and XRechnung tests.

### Added (test infrastructure)
- **`internal/platform/dbtest`** — an integration-test harness that gives each
  test **process** its own schema (`test_<pid>`, dropped on exit), so concurrent
  test binaries never share tables. All packages' `testPool` delegate to it.

### Fixed
- **Database flakiness.** `TestPaymentRunRejectsInvalidCreditorBIC` failed
  intermittently under `go test ./...`: every test binary shared one database,
  `payment_recipient` is keyed by IBAN, `CreateRecipient` upserts the BIC, and the
  fixtures reused a fixed IBAN — so a concurrent binary could overwrite the BIC
  the assertion depended on. Fixed by per-process schema isolation plus unique
  IBAN fixtures (`docs/18-test-isolation.md`).

### Changed
- `contracts/openapi.yaml` gains `401`/`403` responses on every operation.
- `mutation.json` adds `internal/platform/auth` and `internal/platform/tenant`;
  the test harness (`internal/core/golden`) is exempt.
- ADR-0002 and ADR-0003 record that tenancy and auth are now implemented.

## [0.22.0] - 2026-10-26

Contract-first made real: an executable API contract, a generated Angular
client and its first consumer, plus the rest of the reference implementation bundle analysed.

### Added
- **`docs/14-bundle-rest-and-interfaces.md`** — the un-inventoried part of the
  4.2 GB bundle: `the legacy front-end` (the legacy ERP's own front-end, JasperReports + Saiku
  + the `dbt` schema tool), the **SOAP surface** (79 services / 638
  operations, incl. `PaymentExportService`, `CardTerminalService`,
  `RightsAndRolesService`) and the shipped **connectors** (`the card-terminal connector`
  chip-card bridge, `the student-loan client` KfW SOAP client).
- **`tools/gen_openapi.py`** — generates `contracts/openapi.yaml` from the Go
  route table + structs (49 paths, 51 operations, 58 schemas; transitive refs
  resolved, no permissive schemas).
- **`tools/api_contract.py`** — stdlib-only drift guard (`check`), Angular
  client generator (`gen`) and self-test (`selftest`). Fails closed: an
  undocumented Go route or a dangling `$ref` breaks the build.
- **Generated Angular client** `frontend/src/app/core/api/generated/`
  (`models.ts`, `api.service.ts`) from the contract (ADR-0003).
- **First consumer** `frontend/src/app/kasse/cashbook-day.component.ts` — *Kasse:
  open a cash-book day* — with a Karma spec; route `/kasse/tag`.
- **`internal/httpapi/middleware.go`** — correlation id, panic recovery and a
  structured access log (`log/slog`) around the router (NFR-5), with tests.
- **`httpapi.Deps`** — `New` takes a dependency struct instead of thirteen
  positional interfaces; tests supply only what they exercise.
- **Unified error envelope** — every handler now writes the JSON `Problem`
  shape from the contract (`writeProblem`), not plain-text `http.Error`.
- **Nullable fields** — pointer Go fields emit `x-nullable`; the generated
  TypeScript is `number | null`, so the client stops lying about absent values.
- **`docs/15-api-contract.md`** and **`docs/16-http-transport.md`** — how the
  contract pipeline works, and the transport layer that serves it.

### Changed
- `contracts/openapi.yaml` grew from 2 documented routes to all 51; the phantom
  `POST /audit` is gone.
- `tools/run_tests.sh`, `Makefile` (`contracts-check`, `contracts-gen`,
  `contracts-validate`, `quality`) and `.github/workflows/quality.yml` now gate
  the contract (drift guard + redocly lint, via `.redocly.yaml`); generated code
  is excluded from the mutation/policy gates.
- ADR-0003 records that the contract-first pledge is now enforced.

## [0.21.0] - 2026-10-26

KLR: cost centres, cost units and accounting records (issue #20).

### Added
- migration `0015_klr.sql`: `cost_center`, `cost_unit`, append-only `accounting_record`.
- `internal/modules/klr`: domain, repo, audited service (`CreateCostCenter`,
  `CreateCostUnit`, `Post`, `Report`).
- HTTP: create cost centre / cost unit, post a record, read the report.

## [0.20.0] - 2026-10-26

Procurement: purchase orders with budget commitment (issue #18).

### Added
- migration `0014_procurement.sql`: `purchase_order` + `purchase_order_line`.
- `internal/modules/procurement`: domain, repo, audited service with the
  lifecycle `draft → approved → received` (+ `cancelled`), transitions guarded.
- **budget integration**: approval commits the gross to the order's budget
  position (via the budget module); cancelling an approved order releases it.
- HTTP: create/get/approve/receive/cancel purchase orders.

## [0.19.0] - 2026-10-26

Lookup catalogue (issue #17).

### Added
- migration `0013_lookup.sql`: one `lookup_value` catalogue keyed by
  `(namespace, code)` instead of 3,532 key tables.
- `internal/modules/lookup`: domain, repo, audited service (`Upsert`,
  `UpsertMany`, `Get`, `List`, `Seed`); `Seed` loads the standard code lists
  (UNTDID 1001/4461/5305, UNECE units, ISO 4217/3166).
- HTTP: list/get/upsert lookups and `POST /api/lookups/seed`.

## [0.18.0] - 2026-10-26

Budget: positions + postings with Mittelüberwachung (issue #15).

### Added
- migration `0012_budget.sql`: extends `budget_position` with `allocated_cents` +
  description/audit fields and adds append-only `budget_posting`.
- `internal/modules/budget`: domain, repo and audited service; the over-spend
  guard (`CheckAvailable`) rejects a commitment/actual beyond the available amount.
- HTTP: `POST /api/budget/positions`, `GET /api/budget/positions/{id}`,
  `POST /api/budget/positions/{id}/postings`, `GET /api/budget/positions/{id}/balance`.

## [0.17.0] - 2026-10-26

Inbound e-invoice: parse + validate a received XRechnung (issue #13).

### Added
- `internal/core/einvoice.ParseXRechnung`: parse a UBL 2.1 `Invoice` into our
  model and validate it with the same rules used for outbound invoices.
- `InvoiceService.ImportReceived` (audited, `einvoice.receive`) and
  `POST /api/e-invoices/received`.
- round-trip + parser error tests; integration and HTTP e2e tests.

## [0.16.0] - 2026-10-26

E-invoicing: XRechnung 3.0 (UBL 2.1) (issue #11).

### Added
- migration `0011_e_invoice.sql`: `e_invoice`, `e_invoice_party`,
  `e_invoice_line`, `e_invoice_vat` (EN16931-shaped, docs/08 §4).
- `internal/core/einvoice`: the model, EN16931 calculation rules and a
  deterministic **XRechnung 3.0 (UBL 2.1)** generator.
- `internal/modules/einvoice`: atomic persistence + audited service.
- HTTP: `POST /api/e-invoices`, `GET /api/e-invoices/{id}`,
  `GET /api/e-invoices/{id}/xrechnung`.
- golden file validated against the productive `UBL-Invoice-2.1.xsd`.

## [0.15.0] - 2026-10-26

S6 — foundation books (issue #9).

### Added
- migration `0010_foundation_book.sql`: `foundation_book` (one per foundation;
  `UNIQUE (foundation_id, reference)`) and append-only `foundation_booking`.
- `kasse.FoundationBook`/`FoundationBooking`/`FoundationBalance`, a repo and a
  service (`CreateBook`, `Book`, `Bookings`, `Balance`), audited on every write.
- HTTP: `POST /api/foundation-books`,
  `POST|GET /api/foundation-books/{id}/bookings`,
  `GET /api/foundation-books/{id}/balance`.
- **isolation test**: a booking on one foundation's book never appears in
  another's bookings or balance.

## [0.14.0] - 2026-10-26

CAMT.053 statement import (issue #7).

### Added
- **`internal/core/camt053`** — ISO 20022 CAMT.053 parser producing the shared
  parsed-statement shape; `internal/core/bankstmt` holds that neutral shape (the
  MT940 types are now aliases of it).
- `StatementService.Import` detects the format (XML → CAMT.053, else MT940).
- parser unit tests incl. a golden CAMT.053 sample; integration test importing a
  CAMT file and matching a paid order.
- `internal/core/camt053` added to `backend/mutation.json`.

### Changed
- `docs/legacy/kbs-statement-import.md` documents both formats.

## [0.13.0] - 2026-10-26

Statement-line detail: uncleared amount, counterparty, structured `:86:` (issue #5).

### Added
- migration `0009_statement_line_detail.sql`: `bank_statement_line` gains
  `uncleared_cents`, `end_to_end_ref`, `counterparty_name`, `counterparty_iban`,
  `counterparty_bic` (docs/06 §2.3).
- `internal/core/mt940`: parse the structured `:86:` fields in both conventions
  the productive parser supports — numbered (`?30` BIC, `?31` IBAN, `?32`/`?33`
  holder, `?34` purpose) and DK markers (`EREF+`, `SVWZ+`, `ABWA+`, `ABWE+`, …);
  `Line` gains `EndToEndRef` and the counterparty fields.

### Changed
- reconciliation matches on the client reference **or** the end-to-end reference.
- statement lines expose the new fields over HTTP.

## [0.12.0] - 2026-10-26

Split `bank_file` out of `bank_statement` (issue #3).

### Added
- migration `0008_bank_file.sql`: a `bank_file` entity (account, file hash;
  `UNIQUE (account_id, file_hash)`) owns the physical import; `bank_statement`
  references it and the unique key moves to `(bank_file_id, statement_number,
  page)`. Existing rows are backfilled. One file may now hold several statements.
- `kasse.BankFile`; repo `InsertBankFile`/`FindBankFile`/`GetBankFile`;
  `StatementService.BankFile`; `GET /api/bank-files/{id}`.
- `docs/06` §2.1 marked applied; `docs/legacy/kbs-statement-import.md` updated.

### Changed
- `StatementService.Import` is idempotent on the file (via `bank_file`) and
  audits one event per file; `Statement` carries `BankFileID` (was `FileHash`).

## [0.11.0] - 2026-10-26

Rename the own-account entity to `cash_account` (issue #1).

### Changed
- **Breaking (API + schema):** the university's own account entity is renamed
  `bank_account` -> `cash_account`, matching the reference implementation model
  (`docs/06` §2.2) and freeing `bank_account` for the counterparty concept.
  - migration `0007_cash_account.sql` renames the table and its unique
    constraint; the referencing FKs follow.
  - Go: `kasse.CashAccount`, `NewCashAccount`, `CreateCashAccount`,
    `GetCashAccount`, `ErrCashAccountRequired`.
  - HTTP: `POST /api/cash-accounts` (was `/api/bank-accounts`).

## [0.10.1] - 2026-10-26

Vendor model cross-walk.

### Added
- **`docs/06-vendor-model-crosswalk.md`** — a measured comparison of the productive
  the reference implementation finance schema (with its declared FKs and comments) against our
  canonical model and the legacy `kbs` tables. Confirms the Kasse design (our
  table names match the vendor's: `payment_order`, `bank_account`,
  `bankstatementline`, …) and records three refinements to schedule: split
  `bank_file` out of `bank_statement`; keep `cash_account` for the own account and
  `bank_account` for counterparties; add `uncleared_amount` + counterparty
  columns to `bank_statement_line`.

## [0.10.0] - 2026-10-26

Slice S4 — bank statement import (MT940) and reconciliation (Kasse).

### Added
- **Schema** (`migrations/0006_bank_statement.sql`): `bank_statement` (own
  identity: number, page, opening/closing balance — the legacy `kbs.mt940` god
  table repeated the header on every line) and `bank_statement_line` (one row per
  `:61:` with an explicit reconciliation `status` and `order_id`).
- **`internal/core/mt940`** — a SWIFT MT940 parser matching the reference implementation
  parser (`MT940Record`/`Field61`): tags `:20:/:25:/:28C:/:60F:/:61:/:86:/:62F:`,
  the `:61:` field layout, German decimal amounts, `C`/`D`/`RC`/`RD` indicators and
  the `009`/`059` return posting keys.
- **`internal/modules/kasse`** — `StatementService` + `StatementRepo`: parse,
  store, and reconcile each line against the **paid** orders (those in a payment
  run) by order reference, classifying `matched` / `returned` / `unmatched`;
  idempotent on the file hash.
- **HTTP**: `POST /api/bank-statements`, `GET /api/bank-statements/{id}`.
- **Tests**: parser unit tests incl. a golden MT940 sample; integration tests
  (import, reconciliation, idempotency, audit chain); HTTP e2e.
- **`docs/legacy/kbs-statement-import.md`** — S4 legacy findings and field mapping.
- `internal/core/mt940` added to `backend/mutation.json`.

### Quality
- All seven Go mutation targets above the 0.85 floor (`httpapi` 100 %, `kasse`
  99.6 %, `mt940` 92 %); policy-as-code green.

## [0.9.0] - 2026-10-26

Slice S3 — payment run and SEPA credit-transfer export (Kasse).

### Added
- **Schema** (`migrations/0005_sepa_export.sql`): `bank_account` (the debtor
  side, which the legacy `kbs` schema denormalised onto every row),
  `payment_run` (stores the produced `file_xml` and its `file_hash`), and
  `payment_run_item` with `UNIQUE(order_id)` as the double-payment guard.
- **`internal/core/sepa/pain001.go`** — deterministic generator for ISO 20022
  **pain.001.001.09** in the German DK/GBIC profile, matching the productive
  the reference implementation writer (`SEPAWriterCreditTransferVersion39`). Full validation
  (lengths, IBAN, BIC, currency, amounts) and XML escaping.
- **`internal/modules/kasse`** — `PaymentRunService` + `PaymentRunRepo`: build a
  run from approved orders for one debtor account, idempotent on its reference,
  audited; `BankAccount` validation.
- **HTTP**: `POST /api/bank-accounts`, `POST /api/payment-runs`,
  `GET /api/payment-runs/{id}`, `GET /api/payment-runs/{id}/file`.
- **Tests**: generator unit tests incl. a golden file
  (`internal/core/sepa/testdata/golden_pain001.xml`) **validated against the
  vendor's own `pain.001.001.09_GBIC_5.xsd`**; integration tests (run, idempotency,
  double-payment guard, audit chain); HTTP e2e.
- **`docs/legacy/kbs-sepa-export.md`** — S3 legacy findings and the field mapping.

### Quality
- `sepa`, `kasse` and `httpapi` at **100 % mutation efficacy** (gremlins,
  ADR-0008); all six target packages above the 0.85 floor; policy-as-code green.

## [0.8.0] - 2026-10-26

Productive the reference implementation source received and inventoried.

### Added
- **`docs/08-reuse-inventory.md`** — measured inventory of the
  productive the reference implementation build handed over as three archives (full product WAR +
  two hotfixes). Records the module catalogue (341 modules), the vendor data
  model (`the reference schema jar`: 1 486 tables, 1 286 declared FKs,
  German column comments), the config catalogues (3 532 key tables; 1 762
  rights / 7 279 role-rights), the ERP Java sources (`the ERP *`, 1 481 files)
  and the payment-traffic layer (`the reference parser`, 1 651 files) with the
  shipped ISO 20022 XSDs (pain.001/008, camt.052/053/054).
- **`docs/open-questions.md`** — Q1 (which product) answered: the reference implementation is the
  vendor's unified successor; the dump is the legacy ERP. Q5–Q8 updated.

### Notes
- The archives are proprietary and remain uncommitted (AGENTS.md 1/2); they were
  extracted outside the repo. This entry is our analysis citing them.

## [0.7.1] - 2026-10-23

Architecture report: landscape pages, database ER models, more breathing room.

### Added
- **Database ER models in the report** (`tools/gen_erm.py`, `tools/erm.sh`,
  `make erm`). Two families, both generated, never hand-drawn:
  - **target canonical schema** — read *directly* from the SQL migrations
    (`backend/internal/platform/db/migrations/`), so every crow's foot is a
    declared `FOREIGN KEY`.
  - **legacy module schemas** (`sva4`, `mbs`, `cob`, `kbs`) — reconstructed from
    measured metadata (`schema/*.tsv`) and the naming conventions in
    `docs/03-schema-analysis.md`. The legacy schema has **zero** foreign keys, so
    every edge is labelled *inferred*.
- **`--no-erm`** flag and `make c4-pdf-no-erm` for a C4-only build.
- **`tools/test_erm.py`** — self-test guarding the ER generator (entity/FK counts
  stay in sync with the migrations; legacy edges stay labelled *inferred*). Wired
  into `tools/run_tests.sh` and `make test-tools`.

### Changed
- The report (`c4/export/campus-erp-c4-model.pdf`) is now **landscape A4** with
  one diagram per page, generous heading/figure spacing, and two parts —
  *Part I: C4 architecture model* and *Part II: Database ER models*. Previously
  the wide diagrams were shrunk into portrait pages and crowded against the text.

## [0.7.0] - 2026-10-23

Build, packaging and deployment automation.

### Added
- **Top-level `Makefile`** — one entry point for the C4 PDF, backend/frontend
  builds and tests, container images, the local stack, and deployment
  (`docs/build-and-deploy.md` documents every target).
- **C4 → PDF** (`tools/c4_pdf.sh`, `c4/pdf/template.tex`): `make c4-pdf` renders
  `c4/workspace.dsl` to a single PDF (`c4/export/campus-erp-c4-model.pdf`). The
  Structurizr CLI image cannot render images locally ("PNG/SVG export is not
  supported in this build" — that path needs the hosted API), so the script
  exports **PlantUML** and renders it with the `plantuml/plantuml` image, then
  compiles the report with **tectonic**. Both containers run as
  `--user $(id -u):$(id -g)` so the non-root Structurizr container can write.
- **`backend/Dockerfile`** — multi-stage; builds `api`, `worker` and
  `connectors` from one tree into one image (modular monolith, ADR-0004), role
  selected by the container command.
- **`frontend/Dockerfile` + `frontend/nginx.conf`** — Angular production build
  served by nginx, `/api` proxied to the backend on the same origin.
- **`docker-compose.yml` + `.env.example`** — local single-host stack
  (postgres, api, worker, connectors, web). `make up` / `make down`.
- **`deploy/helm/campus-erp`** — Helm chart for Kubernetes (ADR-0007): api,
  worker, connectors, web Deployments, api/web Services, optional Ingress and
  HPA, chart- or externally-managed DSN Secret. Validated with `helm lint` and
  `helm template`.
- **`frontend/proxy.conf.mjs`** — dev-server proxy for `make workon-frontend`
  (iterate on the SPA against a deployed backend; and the reverse via
  `make workon-backend`).

### Notes
- The backend image is verified end-to-end: `docker compose up` applies all four
  migrations and serves `/healthz`; the web container proxies `/api` to the api
  container.

## [0.6.1] - 2026-10-21

### Fixed
- **Flaky integration tests.** Test helpers built unique codes and business dates
  from a modulo-reduced timestamp (`time.Now().UnixNano() % 1000000`). The database
  persists between runs, so values repeated and collided on unique constraints —
  `TestDunningDuplicateLevelIsRejected` failed with a duplicate-level error, and the
  cash-book tests could collide on `business_date`. Uniqueness now comes from a
  database sequence (`testSeqNext`), which is the only source that is unique across
  parallel test packages *and* across runs. Business dates start at year 5000 so a
  freshly created sequence cannot collide with rows from earlier runs.
- Two consequences of that sequence, both hit and fixed while doing it:
  `CREATE SEQUENCE IF NOT EXISTS` is not atomic (parallel packages race on it — the
  same class of bug as the migration race), and a sequence starting at 1 re-issues
  values earlier runs already consumed.

## [0.6.0] - 2026-10-21

Slice S7: tax-office data and foundations.

### Added
- **Canonical schema** (`0004_tax_and_foundations.sql`): `vat_type`, `tax_office`,
  `foundation`.
- **VAT types** replacing `kbs.mwst_art`: integer basis-point rates (0..100 %),
  an enumerated `kind` (output/input) instead of the legacy `mw_typ` character, and
  no sign column — the legacy `mw_vorzeichen` stored `'+'`/`'-'` as text.
- **Exact VAT arithmetic**: `TaxOn`, `GrossFromNet`, `NetFromGross`, `TaxFromGross`,
  with `TaxFromGross` defined as the remainder so **net + tax equals the gross
  exactly** at every rate including 0 % and 100 %.
- **IBAN validation** (ISO 7064 mod-97) for tax-office bank details — the legacy
  `iban varchar(35)` accepted anything (`ErrIBANInvalid`).
- Tax-office and foundation records with real constraints and `(year, number)`
  uniqueness surfaced as a conflict.
- **HTTP API**: `POST /api/vat-types`, `GET /api/vat-types/{code}`,
  `POST /api/tax-offices`, `POST /api/foundations`.
- Tests: domain (VAT arithmetic incl. a rate/amount sweep proving the net+tax
  invariant, IBAN mod-97 with verified fixtures and length boundaries, character
  helper boundaries), service with fakes, Postgres integration, HTTP e2e.
- `docs/legacy/kbs-tax-and-foundations.md` — measured legacy findings behind the
  design.

## [0.5.0] - 2026-10-21

Slice S5: dunning ("Mahnwesen").

### Added
- **Canonical schema** (`0003_dunning.sql`): `dunning_level`, `dunning_band`,
  `dunning_notice`. Replaces the legacy `kbs.mahn`, which hard-codes five bands as
  the column groups `bis_betr1..5` / `mahn_geb1..5` / `prct1..5` — a sixth band
  would be a schema change.
- **Bands are rows**, ordered and validated: they must ascend by upper bound, and an
  open-ended band (bound 0) must be last (`ErrOpenBandNotLast`), so band selection
  is always defined.
- **Integer money arithmetic**: rates are basis points (1/100 %), the charge is
  `fee + round_half_up(amount × rate)`. No decimal floating point anywhere — the
  legacy stores `prct` as `numeric`.
- **Stored charges**: a notice records the charge it was issued with, so a later
  configuration change cannot rewrite history.
- Inactive levels are refused (`ErrLevelInactive`); a duplicate `(year, code)` is a
  conflict (`ErrLevelExists`).
- **HTTP API**: `POST /api/dunning/levels`, `GET /api/dunning/levels/{year}/{code}`,
  `POST /api/dunning/notices`.
- Tests: domain (band selection, rounding boundaries, validation), service with
  fakes, Postgres integration (including a direct-repository test proving the DB
  constraints hold and roll back), HTTP e2e.
- `docs/legacy/kbs-dunning.md` — measured legacy findings behind the design.

### Changed
- `roundDiv` lost an unreachable negative branch (amounts and basis points are
  non-negative by construction); dead code was flagged by mutation testing.

## [0.4.0] - 2026-10-21

Slice S2: payment orders ("Anordnungen") with an enforced four-eyes principle.

### Added
- **Canonical schema** (`0002_payment_order.sql`): `payment_recipient`,
  `budget_position`, `payment_order`, `payment_order_decision`. Replaces the legacy
  `kbs.ao` ~120-column god table, its 16 free-text slots, and the `kbs.zv_freigabe`
  row-clone "approval" (which recorded neither who released an order nor whether the
  releaser differed from the creator). Real foreign keys, integer cents, ISO-4217
  currency — the legacy schema has zero declared FKs.
- **Four-eyes rule**, enforced in the domain: `Approve` refuses when the approver is
  the creator *or* the submitter (`ErrFourEyes` → HTTP 403).
- **Workflow**: draft → submitted → approved / rejected / cancelled, with a mandatory
  reason on rejection and no cancellation of an approved order.
- **Decision trail**: one `payment_order_decision` row per transition, plus a
  hash-chained audit event — every decision is recorded in the current state, the
  history and the audit chain.
- **HTTP API**: `POST /api/payment-orders`, `GET /{id}`, `POST /{id}/{submit,approve,reject,cancel}`.
- **Optimistic locking** on transitions (`SaveOrderStatus` guarded by the status it
  was read at → `ErrOrderConflict`).
- Tests: domain (four-eyes, validation boundaries), service with fakes, Postgres
  integration, HTTP e2e, and a concurrent-migration regression test.
- `docs/legacy/kbs-payment-orders.md` — measured legacy findings behind the design.

### Fixed
- **Migrations are now advisory-locked.** `CREATE TABLE IF NOT EXISTS` is not
  race-free: two migrators starting at once (parallel test packages, or api and
  worker booting together) collided on `pg_type` with a duplicate-key error. Caught
  by the integration tests, covered by a 4-way concurrent regression test.

## [0.3.0] - 2026-10-21

Phase 1 (Kasse) vertical slice: platform foundations + cash-book day lifecycle.

### Added
- **Canonical schema** (`backend/internal/platform/db/migrations/`): append-only
  `audit_event`, `cash_book_day`, `cash_booking`. Database-per-tenant means no
  tenant columns (ADR-0002).
- **Migration runner**: embedded, versioned, idempotent; loadable from any `fs.FS`
  (`db.MigrationsFrom`).
- **Persistent audit store** (`audit.Store`): hash-chained append serialised with a
  PostgreSQL advisory lock, plus `Verify`.
- **Kasse module**: day lifecycle (`open`/`book`/`close`), domain rules, PostgreSQL
  repository, service emitting an audit event per write.
- **HTTP API** (`internal/httpapi`): health, audit verify, and the cash-book routes;
  handlers depend on interfaces so the transport layer is testable with stubs.
- **Tests**: Go unit + PostgreSQL integration + HTTP e2e (axe-free backend e2e via
  `httptest`), plus migration-loader tests.
- **CI**: `go` and `mutation-go` jobs now run against a Postgres service container.

### Fixed
- Audit timestamps are truncated to microseconds before hashing: `timestamptz`
  stores microseconds, so a nanosecond `time.Now()` did not survive the round-trip
  and `Verify` reported a broken chain (caught by the integration test).
- Integration tests no longer share mutable state: they use unique business dates
  and assert audit events per subject, because Go runs packages in parallel.
- gremlins' coverage cache is now isolated **per package** in `tools/run_tests.sh`
  and CI. A shared cache produced wrong, non-reproducible scores (one package
  reported 0 killed after another had run in the same container).
- `httpapi` handlers now share one `statusFor` error mapper (depend on interfaces,
  not the concrete service), which removed the untestable 500-branches and made
  `book`-on-a-closed-day a 409 (conflict) rather than 422.
- A duplicate business date now surfaces as `kasse.ErrDayExists` (409) instead of
  an opaque database error.

## [0.2.1] - 2026-10-20

### Changed
- Repository renamed `legacy-db-reverse-engineering` → `campus-erp`; Go module path
  updated to `github.com/Maximilian-Maag/campus-erp/backend`.
- `AGENTS.md`: corrected the Go mutation command — `./internal/...` is a vacuous pass;
  the runner enumerates packages from `backend/mutation.json`.

### Added
- `docs/plan/roadmap.md` — phase order (Kasse → KLR → Personal → Finanzen → Campus).
- `docs/plan/kbs-pilot.md` — Phase 1 (Kasse) delivery plan: scope, interfaces, target
  schema slice, ETL, slices S0–S8 with acceptance tests, risks, Definition of Done.

## [0.2.0] - 2026-10-20

Target-system design phase: decisions, requirements, C4 and a verified skeleton.

### Added
- **ADRs 0001–0008** (`docs/adr/`): new schema + ETL, database-per-tenant,
  Go + Angular contract-first, modular monolith, connectors isolation, audit &
  compliance, Kubernetes deployment, testing & mutation floor.
- **Requirements** (`docs/requirements.md`) rewritten for the target system:
  functional parity per module, interface parity, compliance, NFRs (15k users,
  peak load), tenancy, migration.
- **C4 model** as a Structurizr workspace (`c4/workspace.dsl`) modelling as-is and
  target; validated with the Structurizr CLI.
- **Backend skeleton** (Go, stdlib-only): `cmd/{api,worker,connectors}`,
  `internal/core/{audit,sepa}`, `internal/platform/config`, module placeholders.
  Unit tests pass; mutation 100% efficacy / 100% coverage on all three target
  packages (gremlins).
- **Frontend skeleton** (Angular 20): tenant resolution service, Karma config with
  a container-safe headless launcher, Playwright + axe e2e, Stryker config.
- **Contracts**: `contracts/openapi.yaml`.
- **Deployment placeholder**: `deploy/README.md`.
- **Quality wiring**: `tools/run_tests.sh`, `backend/mutation.json`,
  `.github/workflows/quality.yml`.

### Fixed
- `policy.json`: ignore Angular's JSONC `tsconfig*.json` (a checker false positive).
- Go mutation runner enumerates packages explicitly — gremlins silently reports
  "No results" for `./...` patterns, which would be a vacuous pass.
- sepa: extracted `isDigit`/`isLetter` helpers so the switch conditions are covered
  by mutation (mutator coverage 80% → 100%).

## [0.1.0] - 2026-10-20

### Added
- Initial reverse-engineering of the legacy university ERP sample dataset
  (`the sample dataset archive`).
- Module catalogue: the reference implementation (campus) vs the legacy university ERP, with module ids from
  `db_version` (`the HR module-GX`, `the finance module-UNIX`/`the finance module-GX`, `the cost-accounting module-GX`, `the cash-book module`).
- Database overview: 1,536 tables / 374 views / 38,453 columns across 4 module
  schemas; framework layer; no foreign keys.
- Schema analysis by module and domain cluster.
- Glossary of German domain terms.
- C4 model (context, container, component, code) as Markdown.
- Reproducible tooling: `scripts/restore.sh`, `scripts/analyze.sh`.
- Generated artefacts under `schema/` (DDL, inventories, keys).
- Policy as code: `tools/policy_check.py`, `policy.json`, pre-commit hook, CI.
