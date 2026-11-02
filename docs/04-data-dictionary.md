# Data dictionary

The data dictionary is generated, not hand-written, because it covers 1,536
tables and 38,453 columns. This document explains the generated artefacts and
how to query them.

## Generated artefacts (`schema/`)

| File | Columns / format | Contents |
|---|---|---|
| `inventory.tsv` | `schema \t table \t comment` | one row per base table (comments empty — see below) |
| `columns.tsv` | `schema \t table \t pos \t column \t type \t length \t nullable` | every column |
| `pks.tsv` | `schema \t table \t pk_columns` | primary key per table |
| `fks.tsv` | `child… \t parent…` | **empty** — the schema declares no foreign keys |
| `tables_<schema>.txt` | one table name per line | per-schema table list |
| `schema_full.sql` | DDL | full `pg_dump --schema-only` |

All of the above are **generated** by `scripts/analyze.sh` from a proprietary
third-party database dump and live in the git-ignored `schema/` directory - they are
not part of this repository.

## Comment coverage is near-zero

The dump contains only **6** column comments and **0** table comments. Any
human-readable meaning must come from naming conventions and the glossary, not
from the database. This is why §5 of the task (documenting semantics) needs
domain input — the DB will not give it up.

## Example queries

```sql
-- all columns of a table
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema='sva4' AND table_name='pbv'
ORDER BY ordinal_position;

-- all k_* catalogues in sva4
SELECT table_name FROM information_schema.tables
WHERE table_schema='sva4' AND table_name LIKE 'k\_%'
ORDER BY 1;

-- all *_to_* link tables across the DB (the only relationship hints)
SELECT table_schema, table_name FROM information_schema.tables
WHERE table_name LIKE '%\_to\_%' ORDER BY 1,2;
```

## Type profile

| Type | Columns |
|---|---|
| `character varying` | 23,384 |
| `integer` | 4,671 |
| `date` | 2,850 |
| `numeric` | 2,584 |
| `double precision` | 1,658 |
| `character` | 1,173 |
| `smallint` | 1,109 |
| `bigint` | 541 |
| `timestamp without time zone` | 325 |
| `text` | 136 |
| `boolean` | 16 |
| `bytea` | 6 |

Notes: `character(n)` (1,173 columns) is legacy fixed-width; `date`-heavy
(2,850) reflects the validity-period style of the data (`dat_von` / `dat_bis`
columns appear in `mbs`).
