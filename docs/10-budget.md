# Budget — Mittelbewirtschaftung

The first non-Kasse ERP slice: budget positions with append-only postings and
**Mittelüberwachung** (a posting may not drive the available amount below zero).
Mirrors the productive model `budget_position` / `budget_posting`
(`docs/08-reuse-inventory.md` §3).

## Model

`budget_position` already existed from the S2 payment-order slice (year, chapter,
title, cost centre/carrier) and is referenced by `payment_order`; migration
`0012` adds the allocated amount and the descriptive fields, and adds
`budget_posting`:

| table | role |
|---|---|
| `budget_position` | a title (Haushaltstitel) with `allocated_cents` |
| `budget_posting` | append-only movements: `commitment` / `release` / `actual` |

## Rule

```
committed = Σ commitment − Σ release
available = allocated − committed − Σ actual
```

A `commitment` or `actual` posting is rejected with `ErrBudgetExceeded` when it
would make `available` negative; a `release` never can. The check is a domain
function (`budget.CheckAvailable`) and is exercised by the over-spend test.

## Mittelherkunft und Bewirtschaftung

Where a position's money comes from and who administers it (migration `0029`, mirroring the vendor's
`budget_source` / `budget_center`, `docs/08-reuse-inventory.md` §3):

- **`budget_source`** — a Mittelquelle: the state's chapter and title, a third-party funder, a
  foundation. It carries a `code` (the identity an import and a person share, unique so the two
  cannot end up as two rows), a name and an optional `chapter`.
- **`budget_center`** — a Bewirtschaftungsstelle: who administers the money. Code and name.
- `budget_position.source_id` / `center_id` — which source a position draws on and which centre
  administers it. **Both are optional**: a position may exist before its funding is settled, and
  the Mittelüberwachung does not depend on either. The vendor's
  `budget_center_budget_position` / `budget_center_budget_source` join tables are flattened onto
  the position, because at this size a position draws on one source and is administered by one
  centre — a join table would carry nothing a column does not.

`AssignPosition(ctx, positionID, sourceID, centerID, by)` names the two halves; a `nil` half
**clears** it, so naming one and clearing the other is one operation rather than two. The position
is read first, so a position nothing carries is a `404` *before* the foreign keys are asked about
the source and the centre; an assignment naming a source or a centre nothing carries is refused as
`ErrAssignmentUnknown` (`422`) rather than as a generic failure, and the foreign key that rejected
it is named in the error. Every write is audited (`budget_source.create`, `budget_center.create`,
`budget_position.assign`), and the audit detail of an assignment states a cleared half as `none`
rather than leaving it out, so "nothing" and "not mentioned" do not read alike.

## Der Budgetimport

A budget year arrives as a file: the Mittelquellen, the Bewirtschaftungsstellen and the
Haushaltsstellen of a year, with each position already saying where its money comes from. `POST
/api/budget/imports` reads such a file and writes it.

**The layout is data, not code** (`internal/core/budgetimport`). The vendor's `budget_import` layout
is unpublished (open question Q8), so the layout shipped here is the target system's own:
`budget-import/v1`, semicolon-separated, with a header line, carrying
`source_code; source_name; source_chapter; center_code; center_name; year; chapter; title;
description; ansatz`. `budgetimport.Layout` names the delimiter, whether there is a header line and
which column carries which field, and `internal/core/budgetimport/testdata/budget-import-v1.csv` is
that layout written down — **replacing the layout for the vendor's export means replacing the
`Layout` value and that sample, not the importing logic.** An operator may name another layout with
the request's `layout`; a layout this build cannot read is refused rather than guessed at.

The rules an import follows:

- **The file is read whole before anything is written.** A file with one unreadable line imports
  nothing and answers `422` naming the line (`cannot read row 37: the Ansatz "zuviel": the amount is
  not a number`), because a half-imported budget is worse than a refused one. A duplicate position
  in one file is refused the same way.
- **The import creates and names; it does not edit.** A code that is already taken is *found*, not
  duplicated, and the name the file carries is used only when the row is created — a name a person
  has corrected is not overwritten by the next import of an older file. The same holds for a
  position's Ansatz and description, and for its assignment: the import names a position that has no
  source or centre, and never replaces one a person set.
- **The codes are the identity**, which is why they are unique and why the file's two codes carry
  their names: the code is what an import and a person share.
- **The Ansatz is euros** in the form a German file writes it (`1.234,56`) or a machine writes it
  (`1234.56`): a comma is the decimal mark, and with no comma a single dot followed by one or two
  digits is one too.
- **One transaction**: sources, centres, positions and their naming land together or not at all.
- The answer distinguishes creating from finding (`SourcesCreated` / `SourcesKept`,
  `CentersCreated` / `CentersKept`, `PositionsCreated` / `PositionsKept`, `PositionsNamed`), because
  a second run of the same file must read as "already there" rather than as fifteen new rows. One
  `budget_import.run` audit event carries those counts and the layout that was read.

The screen's *Budgetdatei einlesen* form does the same: it leaves the format empty (the API reads an
empty format as the standard layout, and the field's placeholder names it), refuses an empty file
before asking, keeps a refused file in the textarea so it can be corrected rather than retyped, and
reads the three lists again after a successful import.

## API

| method | path |
|---|---|
| `POST` | `/api/budget/positions` |
| `GET`  | `/api/budget/positions/{id}` (position + balance) |
| `POST` | `/api/budget/positions/{id}/postings` |
| `GET`  | `/api/budget/positions/{id}/balance` |
| `POST` | `/api/budget/imports` (a budget file; `201` with the counts of the run) |
| `POST` | `/api/budget/sources` |
| `GET`  | `/api/budget/sources` |
| `POST` | `/api/budget/centers` |
| `GET`  | `/api/budget/centers` |
| `POST` | `/api/budget/positions/{id}/assignment` (source and centre; a null half clears it) |

## Screen — *Budget: Mittelherkunft* (`/budget/mittelherkunft`)

Lists the Mittelquellen, the Bewirtschaftungsstellen and the Haushaltsstellen, and creates the first
two. A source's chapter is optional (a funder has no state chapter) and a code that is already taken
is a refusal rather than a merge. The assignment is **one form for the chosen position**, not a form
per row: naming a position is an occasional act, not a per-row one. The three reads are the
server's, a position that is not named yet is shown as `keine` — "no source" and "source cleared"
are the same state and must not read like a missing value — and a named source or centre that is not
in the loaded lists falls back to its id rather than showing nothing.

Every write is audited (`budget_position.create`, `budget_posting.create`,
`budget_source.create`, `budget_center.create`, `budget_position.assign`).

## Not yet done

- ~~`budget_source` / `budget_center`~~ — done since: the two are stored, listed and assigned to a
  position, and the *Mittelherkunft* screen drives them (above).
- ~~The budget import from the vendor model~~ — done since: `POST /api/budget/imports` reads a file
  in the `budget-import/v1` layout, which is **data** (above), so the vendor's own export is a
  `Layout` value plus a sample file away. Reading the vendor's *tables* directly rather than a file
  would go through the ETL datasets (`docs/17-platform-spine.md`), which is a different road to the
  same writes.
- ~~Automatic commitment when a payment order is approved~~ — done since: approving one binds
  its amount on the position it names (`kasse.OrderService.Approve` posts a `commitment` named
  `ANORD:<reference>`), and the binding is refused with `ErrBudgetExceeded` when it exceeds the
  position's available Mittel, so an order cannot be approved beyond its budget. The binding is
  posted **before** the status changes, so a refused approval leaves the order awaiting a
  decision rather than approved without Mittel.
- What follows from that: an order's position must carry Mittel. The order body's
  `budget_position` now takes an `allocated_cents` (docs/15), written when the position is first
  created — naming an existing position does not rewrite its Ansatz, because an order may not
  change the budget it draws on.
- ~~Referencing an existing position created through the budget module (rather than creating one
  inline) is the follow-up~~ — done since: the order body's `budget_position` also takes an `id`,
  which names a position created through the budget module. The position is then used as it
  stands: its Ansatz is not rewritten, and approving the order binds its amount on it like any
  other. The two forms are alternatives — naming an id and describing a position at once is
  refused (`422`, `budget_position_ambiguous`) rather than half-honoured, and an id nothing
  carries is a `404`, not a new position. An id on its own leaves every descriptive field at its
  zero value, which is how the two are told apart.
