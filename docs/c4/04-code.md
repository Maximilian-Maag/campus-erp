# C4 — Level 4: Code (schema)

At C4 level 4 the "code" is the database schema. The full DDL is
`schema/schema_full.sql` (~196k lines). This page shows representative tables to
convey the house style; the complete model is in `03-schema-analysis.md` and the
generated `schema/*.tsv`.

## House style (observed in every module)

- Column names are **prefixed with the table's abbreviation** (`pbv_status`,
  `busa_jahr`, `fibu_…`) — a legacy of the pre-ANSI-join era.
- A surrogate key `*_id` (sequence-backed) is the PK.
- `db_einfuegemarke` on almost every table (audit marker).
- `character(n)` / `character varying` dominate; money is `double precision` or
  `numeric`; dates are `date` / `timestamp without time zone`.

## Representative tables

### `sva4.pbv` — a personnel case (Personal-/Bezüge-Vorgang)

```
db_einfuegemarke   varchar    -- audit marker
pbv_pgd_join_id    integer    -- link to personnel master (pgd)   [no FK]
pbv_nr             smallint
pbv_serial         integer
pbv_status         smallint
pbv_besold_nr      varchar
pbv_rechtsstell    varchar
pbv_art            varchar
pbv_von / pbv_bis  date       -- validity period
pbv_dienstart      varchar
pbv_vertragsdt     date
pbv_vertragsnr     varchar
...
```

Note `pbv_pgd_join_id` — the relationship to the master record is a plain
integer with no foreign key.

### `mbs.fibu_konten` — general-ledger chart of accounts

```
id              integer
sachkonto       varchar    -- G/L account number
bezeichnung     varchar    -- description
haushaltsjahr   varchar    -- fiscal year
klasse          varchar    -- account class
kostenart       varchar    -- cost type
titel           varchar    -- budget line
buchbar         varchar    -- postable (Y/N as text!)
gesperrt        varchar    -- blocked
ust_soll        varchar    -- VAT (as text)
euer            integer    -- income-surplus statement flag
...
```

Booleans are modelled as `varchar` flags (`buchbar`, `gesperrt`) — another
legacy signature.

### `cob.busa` — budget structure line

```
busa_id       integer    -- PK
busa_jahr     smallint   -- year
busa_monat    smallint   -- month
busa_instnr   varchar    -- institution number
busa_projnr   varchar    -- project number
busa_fikrkey  varchar    -- Finanzkreis key
busa_kapitel  varchar    -- budget chapter
busa_titel    varchar    -- budget line
busa_betrag   double     -- amount
busa_quellsys varchar    -- source system  (integration seam)
busa_buchdat  timestamp
...
```

`busa_quellsys` / `busa_quelldatnr` / `busa_quellidnr` show the record keeps its
**source-system provenance** — important for any integration work.

### `kbs.kass` — cash-desk count (denominations)

```
hiskey_id  integer
kass_dat   date
buendel    numeric   -- bundles
bn1000..bn5 numeric  -- banknotes (1000 … 5)
mz10..mz001 numeric  -- coins (10 … 0.01)
geh_sck    numeric   -- cheques
ang_bel    numeric   -- attached receipts
```

A perfect example of the schema encoding **physical cash-counting** semantics
directly as columns.

## Where to go for the rest

| Need | Artefact |
|---|---|
| All columns | `schema/columns.tsv` |
| Keys | `schema/pks.tsv` |
| Per-schema table lists | `schema/tables_<schema>.txt` |
| Full DDL | `schema/schema_full.sql` |
| Module/domain grouping | `03-schema-analysis.md` |
| Term decoding | `glossary.md` |
