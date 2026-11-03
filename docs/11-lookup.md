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

## Imported reference catalogues

Beyond the code-seeded lists, the ETL imports two reference datasets from the
productive source (`docs/19`): the **bank-code directory** (`bank_leit_zahl`, from
`kbs.blz`, 3,732 rows) and the **statutory health insurers** (`krankenkasse`, from
`sva4.k_krankenkasse`, 1,677 rows). Both are plain catalogue rows, so they need no
new table (the point of one catalogue keyed by `(namespace, code)`).

## API

| method | path |
|---|---|
| `GET`  | `/api/lookups/{namespace}` |
| `GET`  | `/api/lookups/{namespace}/{code}` |
| `POST` | `/api/lookups/{namespace}` (bulk upsert) |
| `POST` | `/api/lookups/seed` |

Every write is audited (`lookup.upsert`, `lookup.upsert_many`, `lookup.seed`).

## Code lists and the catalogue

The `vat_category` (UNTDID 5305) and `payment_means` (UNTDID 4461) catalogues are
**generated from the code lists `core/einvoice` validates against** (`VATCategories()`,
`PaymentMeansCodes()`), including their labels. There were never inline enums in the
generator to point at the catalogue — it accepted *any* non-blank code — so the gap was
closed the other way round: the generator now rejects a VAT category or payment means that
EN 16931 does not allow (`ErrVATCategoryUnknown`, `ErrPaymentMeansUnknown`), and the
catalogue is seeded from that same list. One source of truth, pinned by a test that compares
the seeded rows with the code lists in both name and label (`lookup`).

The standard is the authority, not tenant data: the catalogue presents the codes to
operators, but editing a catalogue row cannot make the generator accept a code EN 16931
does not define.

## Not yet done

- Import the full vendor `keytables.xml` where a catalogue is genuinely needed.
