# Legacy findings — bank statement import (Kasse, `kbs`)

Reverse-engineered from the Muster dump `hisrm_muster_30_09_26.backup` (schema
`kbs`) **and** the productive HISinOne source (see `docs/05`). These findings
drive slice S4 and the ETL/interface mapping. Measured, not inferred.

## Legacy `kbs.mt940` — a 97-column god table

`mt940` mixes three concerns on every row:

| concern | columns (examples) |
|---|---|
| statement HEADER | `mt_ausz_nr`, `mt_blatt_nr`, `mt_blz`, `mt_kto`, `mt_anfangsaldo`, `mt_anf_saldodatum`, `mt_endsaldo`, `mt_end_saldodatum` |
| statement LINE | `mt_valuta`, `mt_buchungsdatum`, `mt_sh_kennung`, `mt_umsatzbetrag`, `mt_buchungschl`, `mt_kdref`, `mt_bankref`, `mt_gebuehren`, `mt_ur_betrag`, `mt_grund1` … `mt_grund16` |
| matching | `zj_nr`, `zbu_nr`, `ao_nr`, `jahr`, `ds_nr`, `kap`, `tit`, `dr`, `ut`, `fb`, `ins`, `pers` |

`datmt940` only records which file was read when (`ausz_nr`, `ausz_anfdat`,
`ausz_enddat`, `datname`, `ausz_refid`); `zabe` holds a per-statement balance.
`rueckausz_*` / `rueckfibu_*` hold the return-postings side.

Two structural facts matter for a replacement:

1. **A statement has no identity.** Its header (`mt_ausz_nr`, opening/closing
   balance) is repeated on every line; there is no statement entity to point at.
2. **Reconciliation state is not recorded.** Whether a line was matched to a
   payment, and to which, is not stored, so it cannot be reconstructed.

## The format the vendor actually reads

The productive HISinOne parses MT940 in
`rm.fia.payments.datex.mt940` (`MT940Record` / `Field61` / `Field86`). The
relevant facts, read from that source:

- Tags: `:20:` reference, `:25:` account, `:28C:` number/page, `:60F:`/`:60M:`
  opening balance, `:61:` statement line, `:86:` purpose, `:62F:`/`:62M:` closing
  balance.
- `:61:` layout: value date `YYMMDD`, optional booking date `MMDD`, a debit/credit
  indicator (`C`/`D`, or the reversal forms `RC`/`RD`), an optional currency
  letter (`R` = EUR), the amount, `N`, the three-character posting key, the client
  reference, and an optional bank reference after `//`.
- `:86:` carries the purpose and, often, structured data: the numbered convention
  (`?30` routing/BIC, `?31` account/IBAN, `?32`/`?33` account holder, `?34`
  purpose) or the DK markers (`EREF+` end-to-end, `SVWZ+` purpose, `ABWA+`/
  `ABWE+` differing account holder, `KREF+`/`MREF+`/`CRED+`/`DEBT+`/`COAM+`).
- Amounts use a decimal **comma**.
- Return (back-)postings carry the transaction codes `009` (direct-debit return)
  and `059` (payment-out return).

## What the canonical model does instead (slice S4)

`backend/internal/platform/db/migrations/0006_bank_statement.sql`:

| legacy `kbs` | canonical |
|---|---|
| header repeated on every line | **`bank_statement`** (own identity: number, page, opening/closing balance) |
| 97-column god table | **`bank_statement_line`** — one row per `:61:` |
| 16 free-text slots | one `purpose` text column |
| no reconciliation state | **`bank_statement_line.status`** (`unmatched`/`matched`/`returned`) + `order_id` FK |
| file not modelled | **`bank_file`** — the physical import (file hash), one row per file; statements reference it, so one file may hold several statements |
| re-import unknowable | **idempotent on the file** (`bank_file` unique per account+hash) |

The parser is `backend/internal/core/mt940` (`mt940.Parse`); the service
`kasse.StatementService` stores a `bank_file` (the physical import, keyed by hash)
and its statements, then reconciles each line against the **paid** orders (those
that belong to a payment run), matching on the order reference carried in the
line's client reference. A matched line whose direction is a return is classified
`returned`.

## Verification

- Parser unit tests incl. a golden sample
  (`backend/internal/core/mt940/testdata/mt940_sample.sta`).
- Integration tests: import, reconciliation (matched / returned / unmatched),
  idempotency, audit chain. HTTP e2e for the endpoints.
- `mt940`, `kasse` and `httpapi` above the 0.85 mutation floor (ADR-0008).

## Still open

- **Return postings to FiBu** (the `rueckfibu_*` format) are not produced; the
  ledger is Phase 4 (the pilot keeps only the format in scope).

## CAMT.053 (added)

ISO 20022 **CAMT.053** import is implemented (`internal/core/camt053`); both
formats now feed one reconciliation path via the shared parsed-statement shape
(`internal/core/bankstmt`), and `StatementService.Import` detects the format (an
XML document is CAMT, anything else is MT940). The mapping follows the
productive parser (`rm.fia.payments.datex.sepa.camt053v02/v08`): `Stmt` →
statement, `Bal` `OPBD`/`CLBD` → opening/closing balance, `Ntry` → line, with
`TxDtls/Refs/EndToEndId`, `RltdPties` (`Cdtr`/`Dbtr`) and `RmtInf/Ustrd`.

## CAMT.054 (added)

ISO 20022 **CAMT.054** debit/credit *notification* import is implemented
(`internal/core/camt054`). It is the same entry set as a statement but the message
type carries **no balance**, so the parsed statements hold lines only (both
balances zero). `StatementService.Import` routes an XML document by its container
element — `BkToCstmrDbtCdtNtfctn` → CAMT.054, otherwise CAMT.053 — and reconciles
the entries exactly as CAMT.053. Field mapping follows the productive parser
(`rm.fia.payments.datex.sepa.camt054v02/v08`): `Ntfctn` → statement, `Ntry` → line.

## CAMT.052 (added)

ISO 20022 **CAMT.052** bank-to-customer *account report* (the intraday statement)
is implemented (`internal/core/camt052`). `StatementService.Import` routes an XML
document by container: `BkToCstmrAcctRpt` → CAMT.052, `BkToCstmrDbtCdtNtfctn` →
CAMT.054, otherwise CAMT.053. Unlike a notification, a report *may* carry balances:
`Bal` `OPBD`/`CLBD` are mapped when present and left zero otherwise. Entries
reconcile exactly as the other formats. Field mapping follows the productive parser
(`rm.fia.payments.datex.sepa.camt052v02/v08`): `Rpt` → statement, `Ntry` → line.

All three CAMT parsers read the ISO 20022 `TransactionParties` shape: a party's name
sits under `Pty` (`RltdPties/Dbtr/Pty/Nm`), and the account is a sibling element
(`RltdPties/DbtrAcct/Id/IBAN`), not nested inside the party. The test fixtures are
schema-valid (balances, `Sts`, `BkTxCd`, party/account nesting) and can be checked
against the vendor's XSDs by setting `ERP_XSD_CAMT052` / `ERP_XSD_CAMT053` /
`ERP_XSD_CAMT054` (see docs/20 for the SEPA equivalent).
