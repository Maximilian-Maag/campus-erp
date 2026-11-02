# Legacy findings — cash book (Kasse, `kbs`)

Reverse-engineered from the Muster dump `hisrm_muster_30_09_26.backup`
(PostgreSQL custom archive, `HISKBS 2025.06.0`), schema `kbs`. Row counts are
**measured** (the dump was restored); column detail is from the measured DDL, which
`scripts/analyze.sh` generates locally into the git-ignored `schema/` directory.
Inference is marked *(inferred)*.

This closes the FR-K1 group of `kbs-pilot.md` (§S1) with measured numbers, the way
`kbs-payment-orders.md` closed S2.

## Row counts (Muster dump)

| table | rows | note |
|---|---|---|
| `kbs.schalterbuch` | **0** | counter book (`Schalterbuch`) — the individual cash-desk bookings |
| `kbs.bu` | **0** | bookings (`Buchungen`), the general posting table |
| `kbs.kass` | **0** | daily cash-desk count by denomination |
| `kbs.geing` | **0** | cash receipts (`Geldeingang`) |
| `kbs.dr` | **0** | printing/statement side |
| `kbs.buha` | 0 | booking-hold table |
| `kbs.drprot` | 0 | print change log |
| `kbs.sb` | **1** | **not** a cash-book table — see the correction below |
| `kbs.sbprot` | **2** | change log for `sb` (a_/n_ before-after pairs) |

**Consequence.** As with payment orders, the cash-book tables are structurally
complete but **entirely empty** in the Muster. The schema can be reverse-engineered
with high confidence; the *data* — and with it the mapping rules — cannot. ETL
correctness cannot be validated before a production extract exists; the acceptance
test remains the first university's parallel run (ADR-0001). This is the reason the
cash-book ETL dataset is **not implemented yet** (see `docs/19`).

## Correction: `sb`/`sbprot` are the clerk master, not the cash book

`kbs-pilot.md` groups `sb`, `sbprot` under FR-K1 ("cash book"). They are not:
`kbs.sb` is the **Sachbearbeiter** (clerk / processor) master — one row per user:

| column | meaning |
|---|---|
| `sachb` | clerk id (e.g. `KBS`) |
| `sbbu` | the clerk's booking circle (`Buchungskreis`), shared with `schalterbuch.sbbu` / `bu.sbbu` |
| `sbname` / `sbabt` | name / department |
| `pass_wort` | password (see the security finding below) |
| `freigabe` | release level |
| `tabr_*` | per-area rights (dadr, titkto, dr, buha, mahn, renw, stift, vorg, inst, zawe, pers, finanzamt) |
| `kz_mahnwesen`, `auto_imp*`, `auto_exp*` | dunning flag, automatic import/export flags |

`sbprot` is its before/after change log (`a_*`/`n_*` pairs, `aend_art` I/U). The
`tabr_*` columns are this module's `sv_rechte_sachb` — a solid, if coarse,
per-area rights model (it maps cleanly onto the coarse canonical roles; see
`docs/17`).

## Table shapes — three god tables and a count table

The cash-book tables are the same god-table pattern as `kbs.ao`
(`kbs-payment-orders.md`), each mixing identity, an accounting dimension, recipient
address/bank, SEPA fields and free text:

| table | columns | role |
|---|---|---|
| `kbs.bu` | **152** | bookings — the widest table in the group |
| `kbs.geing` | **115** | cash receipts; booking columns re-prefixed `kbs_*` (a per-process **twin** of `bu`, the same duplication seen with `zv_freigabe`) |
| `kbs.schalterbuch` | **96** | counter book — one row per cash-desk booking (`scha_nr`, `skz_bu`, `betrag`, `scha_dat`, `grund1/2`, `zahlart`, `kz_st`) |
| `kbs.kass` | 30 | daily cash-desk **count** — physical counting encoded as columns: `buendel`/`rollen` (bundles/rolls), banknotes `bn1000`…`bn5`, coins `mz10`…`mz001`, `geh_sck` (cheques), `ang_bel` (attached receipts), plus `soll` (the expected total) and `ver_ers` (replacement) — one row per `kass_dat` |

## No booking-type code list

`kind` (income vs expense) is not readable from the schema: the booking-type codes
`skz_bu` / `kz_bu` / `kbs_kz_bu` / `bu_art` are used across the module (`kz_bu` 6×,
`kbs_kz_bu` 7×, `bu_art` 15×, `zahlart` 50× in the DDL) but there is **no code
table for any of them in `kbs`** — the values live in the application or a shared
key table not present here. With the tables empty, the only candidate signal is the
sign of `betrag numeric(14,2)`, which cannot be confirmed *(inferred, unverified)*.

## Security finding: `sb.pass_wort`

The single `sb` row stores `pass_wort = 'fAWlso22Avod10x5lvsD1jyAlwI='` — a
32-byte-looking base64 value with **no visible per-user salt or KDF marker**. This
is a structural concern worth recording for the replacement: the canonical system
must never store reversible or unsalted secrets, and must delegate authentication
to the IdP (ADR-0003) rather than a local password column. Not reasoned further —
no application code was available.

## Canonical mapping — the counter book is implemented *(inferred)*

The counter book (`kbs.schalterbuch`) is wired as an ETL dataset
(`internal/modules/kasse/etl.go`, `docs/19`). Because the table is empty, the
mapping cannot be data-validated, so it is built explicitly as an inference and
marked here:

| legacy | canonical | rule |
|---|---|---|
| `kbs.schalterbuch` row | `cash_booking` | `betrag`→`amount_cents`, **sign → `kind`** *(inferred: no booking-type code list exists)*, `grund1`+`grund2`→`purpose` (blank → `(ohne Buchungstext)`), `scha_dat`→the day's `business_date` |
| `scha_nr` (+`jahr`) | `cash_booking.legacy_ref` | `kbs.schalterbuch/<jahr>/<scha_nr>`; migration `0016` adds the partial-unique key so the import upserts in place |
| one `scha_dat` | one `cash_book_day` | created **closed** with a zero opening balance; `closing_cents` is computed from the imported bookings (`Balance`) |

The import goes through the day service (`ImportCounterBook`), not raw inserts, so
every write stays on the audit chain (one `cashbook.import` event per day), and the
whole batch is validated before any write. `kbs.kass` (the denomination count) and
`kbs.bu` (ledger-side bookings) are still out of scope: the former needs a data
decision on how a physical count maps onto opening/closing, the latter belongs to
the ledger (Phase 4, `mbs`).

**Still inferred / open:** the sign→kind rule (a zero amount is rejected rather
than guessed), and the `kbs.kass` count mapping. Both need a production extract.
