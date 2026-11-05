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
| `GET` | `/api/dunning/notices/{id}/document` — the notice's stored PDF |

Behind the `erp-kasse` role (`docs/17`). Writes are audited (`dunning.level.create`,
`dunning.notice.issue`); the issue records the document key, so the artefact and the
case are tied together.

The list reads by year then code, and carries each level's bands in their stored
order — including a level with none. The year filter is the server's, so a screen
shows one year's levels rather than holding every year to sift: no `year` lists every
year, and a year nothing was entered for is an empty list, not a 404.

## Not yet done

- **No screen yet.** The routes are reachable from the API; a *Kasse: Mahnwesen*
  screen is the follow-up.
- **No dunning run**: notices are issued per claim, one at a time. Walking the open
  receivables and issuing the next level for each (`next_code`) is a later slice.
- **No ETL dataset**: the legacy dunning tables are thin in the Muster
  (`docs/legacy/kbs-dunning.md`), so the mapping cannot be validated against it.
