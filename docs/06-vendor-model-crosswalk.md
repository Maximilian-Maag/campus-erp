# 06 — Vendor model cross-walk (productive HISinOne ↔ legacy ↔ our canonical)

The productive HISinOne schema is the vendor's **own successor** to the legacy
Hochschul-ERP we replace. It is not our target, but it is the strongest available
reference for how these domains are modelled *correctly*, and it carries declared
FKs and German column comments the legacy dump lacks (`docs/05` §8). This note
records where our canonical schema already agrees with the vendor, and where the
vendor's model suggests a refinement.

Evidence: `cs.sys.dbschema.hisinone.jar` →
`conf/dbconv/conf/sch/ddl.hisinone/experimental/dbmodel.ddl` (1486 tables, with
`COMMENT ON`) and `sch.hisinone/stable/*.fk` (1286 declared FKs). All columns/FKs
below are quoted from those files.

## 1. Headline: the names already agree

For the Kasse domain our canonical table names match the vendor's almost exactly:

| our canonical (this repo) | productive HISinOne | legacy `kbs` |
|---|---|---|
| `payment_order` | `payment_order` | `ao` (god table) |
| `payment_run` | `payment_file` | `zawe` |
| `payment_run_item` | `payment_line` | (part of `ao`/`zawe`) |
| `bank_account` | `cash_account` → `bank_account` | (denormalised onto `ao`/`mt940`) |
| `bank_statement` | `bankstatement` (+ `bankfile`) | `mt940` (god table) |
| `bank_statement_line` | `bankstatementline` | `mt940` (god table) |
| `budget_position` | `budget_position` | `kap`/`tit`/… (flattened) |
| `payment_recipient` | `business_partner` / `bank_account` | `zp_*` (flattened) |

That our independent design landed on the same table names as the vendor is a
useful sanity check.

## 2. Where the vendor is finer-grained — refinements to consider

### 2.1 `bankfile` vs `bankstatement` vs `bankstatementline`

The vendor splits the import three ways:

- `bankfile` — the physical file: `bankfilename`, `file_hash`, `msgid`
  (both unique), `person_id`, `created_at`.
- `bankstatement` — one statement in the file: `bankfile_id`,
  `cash_account_id`, `bankstatement_number`, `bankstatement_year`, opening/closing
  balance; **`UNIQUE (bankfile_id, cash_account_id, bankstatement_number)`**.
- `bankstatementline` — one booking (35 columns), FK → `bankstatement`.

Our S4 model puts `file_hash` on `bank_statement` and keys idempotency on
`(account_id, file_hash, statement_number, page)`. Functionally equivalent for
one-statement files, but the vendor's separate `bankfile` makes **"one file, many
statements"** first-class. *Refinement: extract a `bank_file` entity (file hash,
msgid) and let `bank_statement` reference it.* This also matches the legacy
`datmt940` (which recorded the file, separately from `mt940`).

### 2.2 `bank_account` means the opposite thing

The vendor's `bank_account` is a **counterparty** account — it hangs off
`person_id` / `orgunit_lid` and carries `direct_debit`, `differing_account_holder_*`,
`valid_from/to`. The university's **own** account is `cash_account`, which
*references* a `bank_account` (`bank_account_id`) and points at the FiBu accounts
(`cash_financial_account_id`, `clearing_…`, `suspense_…`, `bank_fees_…`), an
`accounting_unit_id`, a `matching_algorithm_id` and export formats.

Our `bank_account` (S3) is the *own* (debtor) account — i.e. the vendor's
`cash_account`. *Refinement: rename our `bank_account` to `cash_account` (or
introduce both) to avoid the collision, and add the FiBu account wiring
(`financial_account` links) when Phase 4 lands.*

### 2.3 Reconciliation is richer than our three states

`bankstatementline` carries matching fields we do not model: `uncleared_amount`
(partial clearing), `entry_reference`, `collector_reference`,
`accounting_transaction_code`, `prima_nota`, `bank_postingtext_supplement`,
`user_commentary`, plus the counterparty block (`client_iban`, `client_bic`,
`client_name`, `client_bank_account_number`). Our S4 line has only
`status` + `order_id`. *Refinement: add `uncleared_amount` and the counterparty
columns; a partially-cleared line is not the same as a matched one.*

### 2.4 The produced file is a ledger-adjacent document

`payment_file` shares its primary key with `subsidiary_ledger` (`FK id ->
subsidiary_ledger`) — i.e. the SEPA file the vendor produces *is* a FiBu document,
and `payment_line` links each payment to `payment_order` **and**
`subsidiary_ledger`. Our `payment_run` is standalone (Phase 4 will connect it).
*Note for Phase 4: the payment file must become a ledger document.*

### 2.5 Status is a key table, not an enum

The vendor models workflow state as `k_status` (`payment_order.k_status_id FK →
k_status`, `subsidiary_ledger.document_k_status_id`). We use a text enum with a
CHECK. Ours is fine and more explicit; worth noting the vendor's approach is
key-table-driven (matching the legacy `k_*` convention, `docs/03` §0.5).

## 3. The finance model in the vendor's own words (structure)

- `payment_order` (18 cols): `cash_account_id`, `order_number`, `order_date`,
  `pre_notification_date`, `value_date`, `origin_amount`, `paid_amount`,
  `bank_fees`, `request_for_direct_debit_mandate_id`, `k_status_id`.
- `payment_file` (15 cols): `cash_account_id`, `export_file_number`,
  `export_file_reference`, `checksum_amount`, `file_format`, `file_reference`.
- `payment_line` (17 cols): `payment_file_id`, `payment_order_id`,
  `subsidiary_ledger_id`, `paid_amount`, `purpose`, `iban`, `bic`,
  `account_holder`, **`end_to_end_reference`**, `request_collection_date`,
  `sequence_type_k_type_id`.
- `bankstatementline` (35 cols): `valuta_date`, `bank_booking_date`,
  `credit_debit`, `bank_amount`, `bank_booking_key`, `client_reference`,
  `bank_reference`, `original_amount`, `bank_charges`,
  `accounting_transaction_code`, `client_iban`, `client_name`, `uncleared_amount`.
- Double entry: `subsidiary_ledger` (the document: `document_number`,
  `posting_date`, `net_total`, `gross_total`, `previous_/subsequent_…`) and
  `general_ledger` (the postings: `amount_debit`, `amount_credit`,
  `financial_account_id`, `fiscal_year`, `fiscal_month`). `purchase_invoice` /
  `sales_invoice` share the `subsidiary_ledger` PK (`FK id -> subsidiary_ledger`).

Note the direct correspondence with our S3 field mapping: our `end_to_end_id`
= `payment_line.end_to_end_reference`; our statement `client_ref` =
`bankstatementline.client_reference`.

## 4. What this changes

Nothing in the committed slices is wrong; the cross-walk confirms the Kasse
design and yields three concrete refinements to schedule:

1. split `bank_file` out of `bank_statement` (§2.1);
2. rename/duplicate the own-account entity as `cash_account` and keep
   `bank_account` for counterparties (§2.2);
3. add `uncleared_amount` + counterparty columns to `bank_statement_line` (§2.3).

These are recorded here rather than applied silently, because they touch
committed schema (ADR-0001 keeps changes explicit and reversible).
