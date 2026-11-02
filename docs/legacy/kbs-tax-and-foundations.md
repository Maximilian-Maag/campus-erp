# Legacy findings — tax-office data and foundations (Kasse, `kbs`)

Reverse-engineered from `the sample dataset archive` (`the cash-book module 2025.06.0`),
schema `kbs`.

## Row counts

| table | rows | note |
|---|---|---|
| `kbs.mwst_art` | **32** | VAT types — **populated** |
| `kbs.finanzamt` | 0 | tax-office master data |
| `kbs.finanzamtprot` | 0 | tax-office change log |
| `kbs.stift` | 0 | foundations |
| `kbs.stiftprot` | 0 | foundation change log |

`mwst_art` is the only populated table in this group, and it is a useful one: the
VAT catalogue is a **closed, hand-maintained list of 32 German tax categories**,
including the special cases that make a naive rate model wrong.

## `kbs.mwst_art` — the VAT catalogue

| column | type | meaning |
|---|---|---|
| `mw_art` | `varchar(8)` | code |
| `mw_bez` | `varchar(50)` | description |
| `mw_kz` | `numeric` | rate in percent |
| `mw_typ` | `varchar(1)` | `U` / `V` flag |
| `mw_vorzeichen` | `varchar(1)` | `+` / `-` as a **character** |

Observed rows (excerpt):

| `mw_art` | `mw_bez` | `mw_kz` | `mw_typ` | `mw_vorzeichen` |
|---|---|---|---|---|
| 77 | stpf. landw. Ums. (z.B. Wein) nach §24 | 0.00 | | |
| 67 | abziehb. Vorsteuer aus Leistungen §13b | 19.00 | | |
| 48 | stfr. Umsätze ohne Vorsteuerabzug | 0.00 | | |
| 42 | stpf. ig. Erwerb Dreiecksgeschäfte §25b Abs. 2 | 0.00 | U | + |
| 59 | abziehb. Vorsteuer ig. Lieferg. neuer Kfz | 0.00 | V | − |
| 73 | stpf. Umsätze, die Leistungsempf. schuldet (19%) | 19.00 | V | − |

Two findings matter for the canonical model:

1. **The rate alone does not describe the category.** Rows carry 0 % rates while
   being taxable categories (reverse charge, intra-community acquisition), and the
   same 19 % appears for both output and input tax. A model with only a percentage
   cannot represent this catalogue.
2. **The sign is a character, not a number.** `mw_vorzeichen` is `'+'`/`'-'`, and
   `mw_typ` is a single letter — both are effectively enumerations stored as
   strings, with no constraint stopping any other value.

## `kbs.finanzamt` — tax-office master data

`jahr`, `finanzamt_nr`, `bezeichnung`, address (`str`, `plz`, `ort`) and the
office's own bank account as flat columns: `bname`, `inhaber`, `blz`, `konto`,
`bic`, `iban`, plus a `kz_uebern` carry-over flag.

There is **no IBAN validation** anywhere — the column is `varchar(35)` and any
string fits. `kbs.finanzamtprot` records changes with the same `a_`/`n_`
before-after column-pair pattern seen in `kbs.mahnprot` and `kbs.stiftprot`.

## `kbs.stift` — foundations

Four columns: `jahr`, `stift_nr`, `bez` (30 chars), `kz_uebern`. A foundation is
just a numbered fund reference that orders and bookings can be attributed to.

## What the canonical model does instead

Slice S7 (`backend/internal/platform/db/migrations/0004_tax_and_foundations.sql`):

| legacy | canonical |
|---|---|
| `mw_kz numeric` percent | `rate_basis_points` (integer, 1/100 %), constrained to 0..10000 |
| `mw_typ varchar(1)` `U`/`V` | `kind text CHECK (kind IN ('output','input'))` |
| `mw_vorzeichen varchar(1)` `+`/`-` | no sign column — the sign is a property of the arithmetic, not the catalogue |
| `varchar(8)` code, unconstrained | `code text NOT NULL UNIQUE`, validated ≤ 8 chars in the domain (`ErrVATCodeTooLong`) |
| unvalidated `iban varchar(35)` | IBAN validated with ISO 7064 mod-97 (`ErrIBANInvalid`) |
| `kz_uebern char(1)` | `carry_over boolean` |
| `finanzamtprot` / `stiftprot` a_/n_ pairs | one hash-chained `audit_event` per write |

Money arithmetic is exact and pinned:

- `TaxOn(net) = round_half_up(net × rate)`
- `GrossFromNet(net) = net + TaxOn(net)`
- `NetFromGross(gross) = round_half_up(gross × 10000 / (10000 + rate))`
- `TaxFromGross(gross) = gross − NetFromGross(gross)` — defined as the remainder, so
  **net + tax always equals the gross exactly**, at every rate including 0 % and 100 %.
