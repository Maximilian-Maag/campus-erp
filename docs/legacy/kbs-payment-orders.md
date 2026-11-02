# Legacy findings — payment orders (Kasse, `kbs`)

Reverse-engineered from the sample dataset `the sample dataset archive`
(PostgreSQL custom archive, `the cash-book module 2025.06.0`), schema `kbs`.

These findings drive the canonical design in slice S2 and the ETL mapping.
Measured, not inferred — every claim below comes from the restored database.

## Row counts (sample dataset)

| table | rows | note |
|---|---|---|
| `kbs.ao` | **0** | payment order ("Anordnung") |
| `kbs.aoar` | **0** | order type |
| `kbs.zv_freigabe` | **0** | payment-traffic release |
| `kbs.zawe` | **0** | payment instruction / run |
| `kbs.zabe` | **0** | payment run statement |
| `kbs.bank_sctsdd` | 4912 | SEPA direct-debit bank data — **the only populated table** |
| `kbs.mandat` | 0 | direct-debit mandates |
| `kbs.mt940` | 0 | bank statement import |
| `kbs.mahn` | 0 | dunning |

**Consequence.** The sample database is structurally complete but almost entirely
empty for the payment processes. The schema can be reverse-engineered with high
confidence; the *data distribution* cannot. This is the concrete reason ETL
correctness cannot be validated before a production extract exists — the
acceptance test remains the first university's parallel run.

## `kbs.ao` is a ~120-column god table

One row mixes concerns that belong in separate entities:

| concern | columns (examples) |
|---|---|
| identity | `thiskey_id`, `ao_nr`, `jahr`, `vorg_nr` |
| accounting dimension | `kap`, `tit`, `dr`, `ds_nr`, `ut`, `fb`, `ins`, `pers`, `huel`, `ref_huel`, `koart`, `kostelle`, `kotraeger`, `ebene3`, `ebene4` |
| amounts | `betrag`, `fbetrag`, `fkurs`, `gebuehre`, `re_betrag`, `wz_betrag` |
| recipient address | `zp_nr`, `zp_anrede`, `zp_name`, `zp_vorname`, `zp_str`, `zp_plz`, `zp_ort`, `zp_geschlecht`, `zp_landkennz` |
| recipient bank | `zp_blz`, `zp_kto`, `iban`, `bic`, `kontoinhaber`, `kontowbezk`, `bank_name1`, `bank_name2`, `bank_strasse`, `bank_ort`, `bank_landschl` |
| SEPA | `mandat_reference`, `sign_date`, `pre_notification_date`, `sequence_type`, `local_instrument`, `creditor_identification`, `request_collection_date`, `endtoendid`, `purpose_code` |
| dunning | `kz_mahn` |
| dates | `mbs_bu_dat`, `kbs_bu_dat`, `re_dat`, `fa_dat`, `val_dat`, `periode_von`, `periode_bis` |
| free text | `grund1` … `grund16` — **sixteen** columns, each 27 chars |

Additional observations:

- **No foreign keys anywhere** in the legacy schema (0 declared FKs across all
  1,536 tables). Every relationship is application-enforced.
- Amounts are `numeric` with an implicit currency (`wbezk`); there is no
  ISO-4217 code on the row.
- Free text is fragmented into 16 fixed-width slots rather than one field —
  presumably to match a printed form.
- The accounting dimension is *denormalised onto every order*, so a change to a
  cost centre means touching every affected order row.

## The four-eyes release is a row copy — with no audit trail

`kbs.zv_freigabe` has the **same column list as `kbs.ao`** (verified column by
column) plus a handful of extras: `endtoendid`, `kz_meld`, `bank_datname`,
`z_bankkorresp_nr`, `z_bic_korresp_empf`, `z_bic_vermittler`, `purpose_code`,
`ext_dms_id`, `dms_vertrags_id`.

So today's "approval" works by **copying the order row into another table**.
Nothing records:

- who released the order,
- when it was released,
- whether the releaser was a different person from the creator,
- why a release was refused.

A four-eyes control whose only evidence is "a second row exists" is not
auditable — and for a system that must satisfy IDW PS 880 / GoBD, that is the
most important structural finding in this module.

## `kbs.zawe` — payment instruction / run

`zawe` groups payments per bank account and carries running totals:
`blz`, `konto`, `iban`, `bic`, `inhaber`, `bname`, `waehrung`, `art`,
`sperre` (blocked), `stap_nr` (batch), `export_format`, `fibu_konto`,
`bkreis`, and the aggregate pairs
`e_ist`/`a_ist`/`e_soll`/`a_soll`/`be_ist`/`ba_ist`/`be_soll`/`ba_soll`/
`e_schwebe`/`a_schwebe` (income/expense × actual/target/in-flight).

`kbs.zabe` holds the statement side: `zabe_dat`, `zw_nr`, `auszug`,
`azg_datum`, `stand`.

## What the canonical model does instead

Slice S2 (`backend/internal/platform/db/migrations/0002_payment_order.sql`):

| legacy | canonical |
|---|---|
| `ao` god table | `payment_order` + `payment_recipient` + `budget_position` |
| `zv_freigabe` row clone | `payment_order.status` + append-only `payment_order_decision` |
| `grund1..grund16` | one `purpose` text column |
| `betrag numeric` + `wbezk` | `amount_cents bigint` + `currency char(3)` |
| 0 foreign keys | real `REFERENCES` constraints |
| implicit four-eyes (a copied row) | **enforced rule**: `Approve` refuses when the approver is the creator *or* the submitter (`ErrFourEyes` → HTTP 403) |
| no release metadata | `submitted_by`/`submitted_at`, `decided_by`/`decided_at`, `decision_reason`, plus one `payment_order_decision` row per transition and one hash-chained audit event per write |

Every workflow decision is now recorded three times, deliberately: the current
state on the order, the decision history, and the tamper-evident audit chain.
