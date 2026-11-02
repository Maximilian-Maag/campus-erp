# Dump findings — the data and generated layer

`docs/03-schema-analysis.md` reverse-engineered the *table structure* of the
legacy dump. This file records what the **restored dump itself** contains that the
structure alone did not show: the generated function layer, the authorisation
views, and the reference data the ERP actually ships.

## Method

The dump (`the sample dataset archive`, PostgreSQL **custom** format, PG 17.9,
21,537 TOC entries) was restored into a throwaway `postgres:17` container and
queried through the catalog and the data.

```
docker run -d --name hisdump-pg -e POSTGRES_PASSWORD=his \
  -e POSTGRES_DB=hisrm_muster -p 55433:5432 postgres:17-alpine
pg_restore -h localhost -p 55433 -U postgres -d hisrm_muster \
  --no-owner --no-privileges --exit-on-error the sample dataset archive
```

## 1. The dump is far more than tables

| object | count |
|---|---|
| tables | 1530 |
| **views** | **372** |
| sequences | 1114 |
| **functions** | **3594** |
| `COMMENT ON` in the whole dump | **6** |

Functions by schema/language: `sva4` 1709, `mbs` 1247, `cob` 637, `kbs` 1
(all PL/pgSQL except 6 C and 3 SQL). The near-zero comment count confirms the
`docs/04` finding that the DDL carries no documentation.

## 2. The function layer is a *generated* data-access layer

Function names follow a strict per-table convention: `<table><suffix>`, e.g.

| suffix | meaning | examples |
|---|---|---|
| `db` | fetch | `k_mw_artdb`, `bezdb`, `blzdb` |
| `di` | insert | `blzdi`, `plz_formatdi`, `druckdi` |
| `dp` / `dr` / `da` | update / delete | `k_beurlaubartdp`, `dra_workflowdb` |

The application therefore does **not** embed SQL for ordinary reads/writes — it
calls these generated per-table functions. This is why the legacy code shows no
raw SQL and why the dump declares no foreign keys: referential behaviour lives in
the generated layer, not in constraints. `kbs` is the exception (one function,
`gen_awv_fachliche_id`).

A small number of names look like hand-written business logic rather than
generated CRUD (`kontogen_upd`, `_fb_dezkonto_proc`, `k_kostmod_wertedi`,
`stu_booking_responsedi`) and are worth reading when the matching module is ported.

## 3. The `_sbid` views — row-level authorisation

Many of the 372 views are `_<table>_sbid<number>` — one filtered view per table,
keyed by the clerk's organisational unit (Sachbearbeiter-ID, `sbid`). Access is
enforced by *which view* a session reads, not by a `WHERE` clause in the
application. The `_sbid` mechanism also appears in the CM source
(`de.his.appclient.jsf...`). Any replacement must reproduce these semantics; our
tenant/role model (`ERP_TENANT`/`ERP_ROLE`) is the equivalent seam.

## 4. Populated reference data (the "Muster" sample is not empty)

| schema | populated / total tables | rows |
|---|---|---|
| `kbs` (Kasse) | 18 / 94 | 10,078 |
| `sva4` (Personal) | 224 / 636 | 135,814 |
| `mbs` (Finanzen) | 199 / 556 | 78,196 |
| `cob` (KLR) | 99 / 244 | 59,981 |

The populated tables are mostly **reference/master data**, and several are
directly reusable:

| table | rows | contents |
|---|---|---|
| `kbs.blz` | 3,732 | German bank directory: BLZ, BIC, name, place, PAN, IBAN rule |
| `kbs.bank_sctsdd` | 4,912 | **SEPA scheme participation per BIC**: SCT/SDD/COR1/B2B/SCC + validity dates |
| `mbs.bic_sepa_credit_transfer` | 5,834 | BIC directory (credit transfer) |
| `mbs.bic_sepa_direct_debit` | 4,221 | BIC directory (direct debit) |
| `mbs.fibu_konten` | 1,001 | **chart of accounts** (Sachkonto, VAT type, class, Soll/Haben, cost type, budget title) |
| `mbs.fibu_kontenstand` | 2,001 | monthly account balances (Soll/Haben per month 00–13, Saldovortrag) |
| `kbs.basiszins` | 45 | **base interest rate** (§247 BGB) time series from 2002-07-01 |
| `kbs.mwst_art` | 32 | VAT catalogue (documented in `docs/legacy/kbs-tax-and-foundations.md`) |
| `sva4.k_astat_occupation` | 24,451 | occupation / job codes |
| `sva4.k_bw_dienst` | 13,839 | employment (`Dienstverhältnis`) types |
| `sva4.k_krankenkasse` | 1,677 | health insurers |
| `cob.fikr` | 1,000 | FiBu accounting circles with cross-module relations + VAT rate |
| `cob.k_extkotr` | 3,985 | external cost-centre texts |
| `cob.arch_busa` | 9,950 | archived bookings (chapter / title / amount) |

## 5. The ERP describes itself

The dump carries the application's own metadata repository: `db_tabellen` (table
registry with labels), `db_spalten` (columns), and `db_graph_knoten` /
`db_graph_kanten` / `db_graph_joins` (the join graph). This is the source from
which the generated access layer in §2 is built.

## 6. What this means for campus-erp

- **S3 (SEPA export):** `bank_sctsdd` + the `bic_*` directories are the reference
  data for validating that a BIC actually supports the scheme used. We validate
  format only today; scheme validation is a candidate follow-up.
- **FiBu (Phase 4):** `fibu_konten` / `fibu_kontenstand` are the chart of accounts
  and balances to migrate.
- **S5 (dunning):** `basiszins` is the source for default-interest charges.
- **Authorisation:** the `_sbid` view pattern is the legacy row-level access model.
- **Why the dump looks "unconstrained":** no FKs and no SQL in the app, because
  both live in the generated function/view layer, not in the tables.

## 7. Source side — the DAO layer that calls those functions

`docs/05` counted source *files*; counting classes in the shipped jars adds two
modules it under-weighted:

| jar | classes | note |
|---|---|---|
| `the ERP backend.jar` | 1,593 | procurement, invoicing, e-invoicing |
| `the ERP xrechnung.jar` | **1,456** | the XRechnung/ZUGFeRD e-invoicing engine (previously seen only as grep hits) |
| `the ERP legacy.jar` | 81 | the legacy DAO bridge |
| `the ERP frontend.jar` | 0 | resources only |

`the ERP legacy.jar` (`de.his.legacy.persistence.dao.impl.the ERP *`) is the layer
that issues the generated `<table><suffix>` function calls from §2 — it is the
bridge between the Java domain and the legacy database, and is the place to read
when reproducing a legacy read/write path exactly.
