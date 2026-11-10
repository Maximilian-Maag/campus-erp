# 28 — Dunning (Mahnwesen)

A dunning level (Mahnstufe) prices a reminder: how long a claim may run before the
next step, and what the reminder costs — a fixed fee plus interest on the amount
outstanding. Issuing a notice (Mahnung) against a claim charges it once, stores the
computed charge **as it was computed**, and renders it to a PDF (`docs/24`).

It is the counterpart of the collection run (`docs/22`): where a run gathers what is
owed and collects it, dunning is what happens when it is not paid.

## Model

`migrations/0003_dunning.sql` and `0019_dunning_notice_document.sql`:

| table | role |
|---|---|
| `dunning_level` | a stage per business year: `code`, `description`, `days_allowed`, soft `active`, `next_code`, `UNIQUE (year, code)` |
| `dunning_band` | the amount bands of a level: `position` (ordered), `up_to_cents`, `fee_cents`, `rate_basis_points` |
| `dunning_notice` | one issued notice: `claim_reference`, `level_code`, `outstanding_cents`, the stored `charge_cents`, `issued_on`, `issued_by`, `document_key` |

The legacy table (`kbs.mahn`) flattened five bands into columns (`bis_betr1..5`,
`mahn_geb1..5`, `prct1..5`) and kept rates as `numeric` percentages with no ordering
constraint. The canonical model puts the bands in their own table — any number of
them — and keeps rates as **integer basis points** (1/100 percent), so money
arithmetic stays exact. A level can be retired without deleting it; there are no hard
deletes (ADR-0006).

## The charge

`kasse.DunningLevel.Charge` picks the band whose `up_to_cents` first reaches the
outstanding amount (an `up_to_cents` of `0` is open-ended and must be the last band)
and returns:

```
charge = fee_cents + round_half_up(outstanding_cents × rate_basis_points / 10000)
```

A level validates its own shape before anything is written: a code is required, at
least one band, bands ascending by upper bound with an open-ended band last and no
negative values. These are sentinel errors matchable with `errors.Is`
(`ErrLevelCodeRequired`, `ErrBandsRequired`, `ErrBandsNotAscending`,
`ErrOpenBandNotLast`, `ErrNegativeBand`).

The charge is **computed once and stored**, not re-derived from today's bands: a
configuration change must not silently rewrite what a notice charged. A notice against
an unknown level is `ErrNotFound`; against an inactive level, `ErrLevelInactive`.

## API

| method | path |
|---|---|
| `POST` | `/api/dunning/levels` — create a level with its bands (201) |
| `GET` | `/api/dunning/levels` — list them with their bands, `?year=` to narrow |
| `GET` | `/api/dunning/levels/{year}/{code}` — read one |
| `POST` | `/api/dunning/notices` — issue a notice for a claim (201) |
| `POST` | `/api/dunning/runs` — advance the open receivables due by a date one step (200) |
| `GET` | `/api/dunning/notices/{id}/document` — the notice's stored PDF |

Behind the `erp-kasse` role (`docs/17`). Writes are audited (`dunning.level.create`,
`dunning.notice.issue`, `dunning.run`); the issue records the document key, so the
artefact and the case are tied together.

The list reads by year then code, and carries each level's bands in their stored
order — including a level with none. The year filter is the server's, so a screen
shows one year's levels rather than holding every year to sift: no `year` lists every
year, and a year nothing was entered for is an empty list, not a 404.

*Kasse: Mahnwesen* (`/kasse/mahnwesen`) drives all of it: it lists the selected year's
levels with their bands, defines one (bands entered as rows) and issues a notice
against a claim, showing the charge that came back. Amounts are typed in euros and a
band's rate in percent — the two conversions sit in `core/format` with the cents
pair — and the level's year and code travel from the picker, so a notice is always
issued against a level that is on screen. A notice's PDF is linked by its document
route rather than read through the generated client, which reads that route as text;
the link appears only when the notice actually carries a document (`docs/24`).

## The run

`POST /api/dunning/runs` `{ "year": 2026, "as_of": "2026-03-01" }` walks the open
receivables and advances each claim **one** step along the year's chain:

1. The year's levels are resolved into one chain: a level names its successor in
   `next_code`. Exactly one level must be the first (no other level names it). A
   `next_code` that names no level of the year ends the chain there, so a level can
   be configured before its successor; several first levels, a cycle, or a level the
   first cannot reach is refused (`409`) rather than guessed at.
2. The open receivables due on or before `as_of` are selected
   (`OpenReceivablesDueBy`), and each claim's most recent notice is read. A claim
   that has never been dunned starts at the chain's first level; otherwise the next
   level is its latest notice's level's `next_code`.
3. A level's `days_allowed` is its grace period, counted from the previous step —
   the item's due date for the first level, the notice's `issued_on` afterwards. A
   step whose grace period has not elapsed is skipped as `not_due`.
4. A claim is dunned at most once per as-of date: a notice issued on or after it is
   skipped as `already_dunned`, so running the same day again is a no-op rather than
   a second notice.
5. The notice goes through the same dunning service the screen uses, so it is
   charged, rendered to a PDF and audited exactly like a hand-issued one. A failed
   issue is reported as `issue_failed` with its detail, and the run continues with
   the other claims.

The report lists every receivable the run looked at: for an issued one the level,
charge, document and notice id; for a skipped one the reason (`not_due`,
`already_dunned`, `unknown_level`, `no_next_level`, `next_level_missing`,
`issue_failed`). The run appends one `dunning.run` audit event with the counts.

A run's notices carry the claim reference `receivable/<id>`: the open item's id is
stable and unique, so the notice stays traceable to the item without a second
identifier on it.

*Kasse: Mahnlauf* (`/kasse/mahnlauf`) takes the year and the as-of date, starts the
run and renders the report — what the run returned is what the operator sees, no
browser-side filtering.

## Not yet done

- **No ETL dataset**: the legacy dunning tables are thin in the Muster
  (`docs/legacy/kbs-dunning.md`), so the mapping cannot be validated against it.
