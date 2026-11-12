# 29 — Foundations (Stiftungen)

A foundation (Stiftung) is money held for a purpose — a stipend, a prize, a bequest. Slice S6
gives each foundation **its own book**, so one foundation's movements can never be mixed with
another's, and keeps every movement append-only with the balance **derived** from them.

This document covers the whole slice: the model, the reads the *Finanzen: Stiftungen* screen
needed on top of it (`docs/15`), and what is still open.

## Model

`migrations/0004_tax_and_foundations.sql` and `0010_foundation_book.sql`:

| table | role |
|---|---|
| `foundation` | the reference itself: `year`, `number`, `description`, `carry_over`, `UNIQUE (year, number)` |
| `foundation_book` | one foundation's own book: `foundation_id`, `reference`, `currency`, `created_by`, `UNIQUE (foundation_id, reference)` |
| `foundation_booking` | one append-only movement: `book_id`, `kind` (`income`/`expense`), `amount_cents`, `purpose`, `booked_by` |

The legacy schema (`kbs.stift`) is foundation master data only: the movements live in a general
cash/payment table and "which foundation" is a column convention. The canonical model makes the
separation structural — a book belongs to exactly one foundation (`REFERENCES foundation(id)`)
and a movement to exactly one book — so isolation is enforced by the schema rather than by every
query remembering a filter. Amounts are integer cents, `kind` is a closed set, and nothing is
updated or deleted (ADR-0006).

## Reads and writes

| method | path |
|---|---|
| `POST` | `/api/foundations` — enter a foundation reference (201) |
| `GET` | `/api/foundations` — list them, `?year=` to narrow |
| `POST` | `/api/foundation-books` — open a book for a foundation (201) |
| `GET` | `/api/foundations/{id}/books` — list one foundation's books |
| `POST` | `/api/foundation-books/{id}/bookings` — post a movement (201) |
| `GET` | `/api/foundation-books/{id}/bookings` — list a book's movements, oldest first |
| `GET` | `/api/foundation-books/{id}/balance` — a book's derived balance |

Behind the `erp-finanzen` role (`docs/17`). Writes are audited (`foundation.create`,
`foundation_book.create`, `foundation_booking.create`).

The foundation is a **path segment** in `/api/foundations/{id}/books`, not a query: one
foundation's books are the only thing that route can answer, so the isolation cannot be lost to a
forgotten filter. A foundation nothing carries is an empty list rather than a `404` — the list of
foundations is what says which exist — and a path that is not an id is refused before any read.

The year filter on the list is the server's, like the dunning levels (`docs/28`): no `year` lists
every year, and a year nothing was entered for is an empty list.

A book's `reference` is unique per foundation and a duplicate is a `409`, not a second book. A
movement must be positive, carry a purpose and a known `kind`; a movement against an unknown book
is a `404`, decided by the foreign key and translated by the repository.

The balance is **derived**: income and expense are summed from the movements on every read, so
there is no stored figure that can drift. The net is the difference of the two; the screen shows
all three.

## The screen

`frontend/src/app/finanzen/foundations.component.ts` — *Finanzen: Stiftungen* (route
`/finanzen/stiftungen`).

It reads in one direction — foundation, book, movements — because that is the model: a book
belongs to one foundation and a movement to one book, and each level is asked of the server for
the level above it rather than filtered in the browser. So nothing here can show one foundation's
books under another's name, and no screen has to reproduce the isolation rule.

The year is sent as typed and parsed by the server, which is the only side that knows which years
exist; a year the server will not read comes back as a **failed load**, not as an empty list —
"no foundation in this year" and "this is not a year" must not look the same. After a foundation
is created the list is read again rather than appended to, because the list on screen is the
server's answer to a filter the new row may not match. A new book or movement, by contrast, joins
the list it was read for.

The balance is read again after every movement instead of being adjusted in the browser, for the
same reason it is derived server-side. Amounts are typed in euros and converted once through
`euroToCents` — the inverse of the `formatCents` every figure is shown with (`core/format.ts`) —
and a movement is always booked against the book that is on screen.

## Closing a year

`POST /api/foundation-books/{id}/carry-over` posts a book's net as the opening movement of the same
foundation's book in the following year. The body may name the next book's reference; an empty one
reuses the source's.

The rules, in the order they refuse, each with its own machine code so a client can say what to do:

| refusal | code | why |
| --- | --- | --- |
| the foundation does not carry | `kasse.carry_over_not_allowed` | `carry_over` is false: the money is spent in its year |
| nothing to carry | `kasse.carry_over_nothing` | the net is zero |
| the next year is not open | `kasse.carry_over_target_missing` | no `foundation` row for `year + 1` with the same number |
| the next book already carries movements | `kasse.carry_over_target_not_empty` | a second carry-over would count the money twice |

Two decisions are worth stating. **The next year's foundation record is not created here**: master
data is not a side effect of a posting, so the operator opens the year first and the refusal names
the year and number that are missing. And **the source book is not touched**: the years' books are
append-only records of what happened, so closing a year means opening the next with the net, not
editing the one that ended. A deficit carries as an `expense` of the absolute amount, so the next
book opens with the same net the source closed with.

The screen offers it per book (*Ins Folgejahr übertragen*) and names the refusal it gets back
rather than a generic failure, because the four refusals ask for four different things.

## Correcting a movement

A book's movements are append-only: a booking made in error is answered by a **counter-movement**,
never by an edit or a delete. `POST /api/foundation-bookings/{id}/reverse` appends a movement of the
same amount and the opposite kind, pointing back at the original, and marks the original as
cancelled by it. Both halves are written in one transaction, so a movement is never marked cancelled
without its counter-movement.

The two columns are `reverses` (this one cancels that one) and `reversed_by` (that one cancels this
one), with a partial unique index on each, so "cancelled" is a stored fact rather than a flag that
can drift, and a second cancellation cannot slip through between two connections — whichever
transaction loses reports `kasse.booking_already_reversed` (409) rather than appending a second
counter-movement.

| refusal | code | status |
| --- | --- | --- |
| the movement does not exist | `kasse.booking_not_found` | 404 |
| it has already been cancelled | `kasse.booking_already_reversed` | 409 |
| it *is* a counter-movement | `kasse.reversal_not_reversible` | 422 |

The last one is a deliberate limit: cancelling a cancellation re-instates the original, which is a
different act with its own wording on a statement, and nothing here supports it. The reason given
travels in the counter-movement's purpose (`Storno zu Buchung 7: doppelt erfasst`), so the statement
explains itself months later without a join. The balance is derived from the movements, so a
cancelled pair nets to zero without either row being touched, and the screen shows the original as
*storniert* with the counter-movement marked *Gegenbuchung*.

The screen reads those codes through `core/problem.ts` (`problemCode`), which answers "no code" for
anything that is not the API's problem body — the API wraps a refusal as `{ error, code }`, Angular
wraps *that* in the response's `error`, and a failure that never reached the API (a dropped
connection, a proxy's HTML page) carries neither. Reading `code` off such a reply would throw while
the screen is trying to explain what went wrong; every branch of that reader is unit-tested for
exactly that reason.

## Not yet done

- **No ETL dataset.** The legacy foundation tables are thin in the Muster
  (`docs/legacy/kbs-tax-and-foundations.md`), so the mapping still cannot be validated against a
  production extract (`docs/19`).
