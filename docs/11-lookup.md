# Lookup catalogue

The productive source has **3,532 separate key tables** (one per coded value set,
`docs/08-reuse-inventory.md` §5) — a new catalogue means a new table, a new join,
a new DAO. campus-erp keeps **one** catalogue keyed by `(namespace, code)`; a new
value set is rows, not a table.

## Model

`lookup_value` (`migrations/0013_lookup.sql`):

| column | role |
|---|---|
| `namespace` | the value set (e.g. `vat_category`) |
| `code` | the code (e.g. `S`) |
| `short_text` / `long_text` | labels |
| `sort_order` | display order |
| `active` | soft retirement |

`UNIQUE (namespace, code)`, so `Upsert` is idempotent (insert or update in place).

## Seeded catalogues

`Seed` loads the standard code lists the productive source itself references:
document type (UNTDID **1001**), payment means (UNTDID **4461**), VAT category
(UNTDID **5305**), unit of measure (UNECE Rec. 20), currency (ISO **4217**) and
country (ISO **3166-1**). Seeding is idempotent.

## API

| method | path |
|---|---|
| `GET`  | `/api/lookups/{namespace}` |
| `GET`  | `/api/lookups/{namespace}/{code}` |
| `POST` | `/api/lookups/{namespace}` (bulk upsert) |
| `POST` | `/api/lookups/seed` |

Every write is audited (`lookup.upsert`, `lookup.upsert_many`, `lookup.seed`).

## Not yet done

- Point the existing enums (VAT types, payment means in the e-invoice generator)
  at the catalogue instead of inline constants.
- Import the full vendor `keytables.xml` where a catalogue is genuinely needed.
