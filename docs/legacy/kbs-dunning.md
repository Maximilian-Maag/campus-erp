# Legacy findings — dunning (Kasse, `kbs`)

Reverse-engineered from `hisrm_muster_30_09_26.backup` (`HISKBS 2025.06.0`),
schema `kbs`. Measured against the restored database.

## Row counts

| table | rows | note |
|---|---|---|
| `kbs.mahn` | **0** | dunning levels |
| `kbs.mahnprot` | **0** | dunning level change log |
| `kbs.feao` | 0 | dunning-related order flag |
| `kbs.abrn` | 0 | settlement run |

As with payment orders, the Muster dump carries the structure but no rows.

## `kbs.mahn` — five bands flattened into the row

A dunning level ("Mahnstufe") stores its amount bands as **hard-coded column
groups**, five of them:

| band | upper bound | fee | rate |
|---|---|---|---|
| 1 | `bis_betr1` | `mahn_geb1` | `prct1` |
| 2 | `bis_betr2` | `mahn_geb2` | `prct2` |
| 3 | `bis_betr3` | `mahn_geb3` | `prct3` |
| 4 | `bis_betr4` | `mahn_geb4` | `prct4` |
| 5 | `bis_betr5` | `mahn_geb5` | `prct5` |

Plus the level's own attributes: `jahr`, `mahn` (the level code), `bez`
(description, 30 chars), `anz_tag` (days allowed), `kz_mahn` (active),
`nxt_mahn` (next level), `kz_uebern` (carry-over flag), and a claim reference
`ford_nr` / `ford_dat` / `ford_art`.

Consequences of the fixed five columns:

- A sixth band is a schema change.
- The bands have **no ordering constraint** in the schema — nothing stops
  `bis_betr1` from being larger than `bis_betr2`, and nothing requires the last
  band to be open-ended. Band selection is therefore undefined for an amount that
  falls outside every bound.
- `prct1..5` are `numeric` percentages, so the charge is computed in decimal
  floating point.

## `kbs.mahnprot` — a hand-rolled change log

`mahnprot` duplicates **every** field of `mahn` twice: `a_<col>` for the value
before and `n_<col>` for the value after, plus `aend_dat` (date), `aend_zeit`
(time as an 8-char string), `aend_art` (change type) and `aend_sachb` (the clerk).

So the legacy system *does* keep configuration history — but with the
before/after pattern hard-coded per column, which is exactly what a generic
append-only audit record (our `audit_event`) expresses once for every entity.
`kbs.stiftprot` and `kbs.finanzamtprot` repeat the same pattern for foundations
and tax offices.

## What the canonical model does instead

Slice S5 (`backend/internal/platform/db/migrations/0003_dunning.sql`):

| legacy | canonical |
|---|---|
| `bis_betr1..5` / `mahn_geb1..5` / `prct1..5` | `dunning_band` rows — any number of bands |
| no ordering constraint | bands must ascend by upper bound; an open-ended band (0) must be last (`ErrOpenBandNotLast`) |
| `prct` as `numeric` percent | `rate_basis_points` (integer, 1/100 %) — exact integer money arithmetic |
| undefined band selection | explicit rule: the first band whose bound covers the amount, open-ended otherwise; no match is an error (`ErrNoDunningBand`) |
| `mahnprot` a_/n_ column pairs | one hash-chained `audit_event` per configuration change |
| charge recomputed from live config | the charge is **stored on the notice**, so later config changes cannot rewrite history |
| `kz_mahn` as a 1-char flag | `active boolean`, and an inactive level is refused (`ErrLevelInactive`) |

The rounding rule is pinned: the rate is applied with half-up rounding away from
zero, so `12 550` cents at 1 % charges `126`, not `125`.
