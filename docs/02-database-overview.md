# Database overview

Reverse-engineered from `hisrm_muster_30_09_26.backup`. All numbers below were
measured on a restored copy (PostgreSQL 16 sandbox; dump produced by 17.9).
Regenerate with `scripts/analyze.sh`.

## 1. Physical characteristics

| Metric | Count |
|---|---|
| Schemas (user) | 8 (`sva4`, `mbs`, `cob`, `kbs`, `hisrm`, `public`, `histest`, `test`) |
| Base tables (`relkind='r'`) | 1,536 |
| Views | 374 |
| Sequences | 1,117 |
| Functions | 3,601 |
| Triggers (non-internal) | 1,439 |
| Indexes | 4,352 |
| Columns (user tables) | 38,453 |
| Primary keys | 1,558 |
| Unique constraints | 49 |
| Check constraints | 2 |
| **Foreign keys** | **0** |

### No declared foreign keys

There are **zero** `FOREIGN KEY` constraints. Relationships are enforced in the
application layer. This is the single most consequential fact for any
reverse-engineering or data-integration effort: **referential integrity must be
inferred from naming and shared columns, and cannot be trusted to the DB.**

### Tables carrying data

All 1,536 tables were dumped with a `TABLE DATA` section, but only ~546 contain
rows in this Muster (seed) dataset; the remainder are empty. Largest tables:

| Schema | Table | Rows (est.) | Likely meaning |
|---|---|---|---|
| `sva4` | `__k_plz` | 35,941 | postal-code catalogue |
| `cob` | `k_plz` | 33,853 | postal-code catalogue |
| `mbs` | `__k_plz` | 33,853 | postal-code catalogue |
| `sva4` | `k_astat_occupation` | 24,451 | occupation catalogue (statistics) |
| `sva4` | `__r_wof` | 23,185 | workflow relation |
| `sva4` | `k_bw_dienst` | 13,839 | service/grade catalogue |
| `cob` | `arch_busa` | 9,950 | archived budget structures |
| `mbs` | `bic_sepa_credit_transfer` | 5,834 | SEPA credit transfers |
| `kbs` | `bank_sctsdd` | 4,912 | SEPA direct-debit bank data |

## 2. Schema = module, and each module is self-contained

Each module schema is effectively its own application database. It carries **its
own copy** of the HIS runtime framework, not a shared central one. So
`sv_vorgangname` exists separately in `sva4`, `mbs` and `cob`; `inst` exists in
four schemas; `db_version` exists in all four. Cross-schema tables are
duplicated structures, not views onto one shared table.

This has a direct consequence: the same logical entity (e.g. an institution,
a postal code) is replicated per module, and keeping copies in sync is a
job of the application / integration layer.

### The framework layer (present in every module schema)

| Prefix / name | Role |
|---|---|
| `sv_*` | Core runtime: transactions/cases (`sv_vorgang*`, `sv_teilvorgang`), menus (`sv_menu_*`), rights (`sv_rechte_sachb`, `sv_sachb_info`), configuration (`sv_konfig_*`), app context (`sv_appcontext`), integration (`sv_integration`) |
| `wof`, `__wof`, `r_wof` | Workflow engine (Wof = Workflow) + authorisations |
| `workflow_*` | Workflow use-case / responsible-person plumbing |
| `inst`, `inst_to_ext`, `inst_uebinst` | Institutions / organisational units (`Einrichtung`) and their hierarchy |
| `db_tabellen`, `db_spalten`, `db_graph_*` | Metadata-driven UI: table/column registry and the "graph" navigation model |
| `k_*` | Catalogues / reference data (`Katalog`) — 200+ in `sva4` alone |
| `r_*` | Rules / relations |
| `rev_*` | Revision / audit data |
| `imp_*` | Import staging |
| `out_*` | Export views/tables |
| `*_to_*` | Many-to-many link tables (e.g. `ansc_to_inst`, `bga_to_mbs`, `proj_to_inst`) |

### The `__` prefix convention

Newer modules carry a `__`-prefixed variant alongside the plain name
(e.g. `__k_plz` next to `k_plz`, `__r_wof`, `__wof`, `__ansc`). Counts:

| Schema | `__`-prefixed | plain framework |
|---|---|---|
| `sva4` | 165 | 178 |
| `mbs` | 56 | 43 |
| `cob` | 7 | 50 |
| `kbs` | 0 | 1 |

Observed as a **generation split**: the `__` tables are the newer-generation
framework copies; both generations can coexist during a migration. Do not treat
`__k_plz` and `k_plz` as the same table without checking which one is live for a
given module.

## 3. Audit & history

Audit is implemented with **triggers and marker columns**, not FKs:

- A `db_einfuegemarke` column is present on **1,595** tables (insert marker).
- Per-table trigger functions named `<table>db`, `<table>da`, `<table>ib`,
  `<table>ub` (delete/insert/update, before/after) — 1,439 triggers total,
  3,601 functions.
- `__account_history` / `account_history` tables record change history.

Implication: history and audit data is spread across per-table trigger logic and
`*_history` tables; there is no single audit table.

## 4. Naming conventions (domain)

- Table/column names are **German, heavily abbreviated** (legacy DB2/Oracle-era
  limits): `busa` = Budgetstruktur, `bzgr` = Bezugsgröße, `fikr` = Finanzkreis,
  `gege` = Gegenstand/Geschäftsvorfall, `kass` = Kasse, `mahn` = Mahnung,
  `stift` = Stiftungs…, `zawe` = Zahlweg, `blz` = Bankleitzahl, `plz` = Postleitzahl.
- See [`glossary.md`](glossary.md).
- Catalogues are almost always `k_<something>`; imports `imp_*`; exports `out_*`.

## 5. Version / module identification

Each module schema has a `db_version` table listing the HIS system id and
version. This is the authoritative way to identify which product module a schema
belongs to. Example:

```
$ select * from kbs.db_version;
 hiskey_id | his_system | version   | kern_system
        36 | HISKBS     | 2025.06.0 | 1
```

See `01-module-catalogue.md` for the full mapping.

## 6. How to read the artefacts

| File | Contents |
|---|---|
| `schema/schema_full.sql` | full `pg_dump --schema-only` DDL (~196k lines) |
| `schema/inventory.tsv` | `schema \t table \t comment` for all 1,536 tables |
| `schema/columns.tsv` | every column: `schema, table, pos, name, type, length, nullable` |
| `schema/pks.tsv` | primary-key columns per table |
| `schema/fks.tsv` | foreign keys — **empty by design** (none exist) |
| `schema/tables_<schema>.txt` | plain list of tables per schema |
