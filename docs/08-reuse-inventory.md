# Reuse inventory — what in the HISinOne sources is usable for campus-erp

This is the "everything we can mine" pass over the three archives in
`~/Downloads` (productive HISinOne source + the legacy ERP dump). It indexes the
reusable assets and what each is good for. Companion docs: `docs/05` (source
inventory), `docs/06` (vendor ↔ canonical cross-walk), `docs/07` (dump data +
generated layer), `docs/03` (legacy schema).

Extracted trees (proprietary — kept out of this repo, per `AGENTS.md`):

| tree | size | contents |
|---|---|---|
| `~/Work/hisinone/src/dbschema` | 39 MB | the vendor **model** (`sch.hisinone/*.tab\|.fk\|.idx`, `view.hisinone`, `scr.hisinone`, `ddl.hisinone`) |
| `~/Work/hisinone/src/h1/2026-08-23` | 168 MB | qisserver source, **17,571 .java** |
| `~/Work/hisinone/src/hotfix1006` | 350 MB | the ERP/e-invoicing jars + webapp + `keytables.xml` |
| dump `hisrm_muster_30_09_26.backup` | 15 MB | legacy ERP DB (restored on `:55433`) |

## 1. The vendor model is machine-readable — this is the single best asset

The vendor ships its whole relational model as text, *with German comments*:

```
…/dbschema/META-INF/resources/conf/dbconv/conf/sch/sch.hisinone/stable/
  1458 × *.tab   CREATE TABLE, per-column {German comment}
  1286 × *.fk    ALTER TABLE … FOREIGN KEY   (2943 FKs in total)
  1447 × *.idx   CREATE INDEX / CREATE UNIQUE INDEX
```

This closes the two gaps the legacy dump left open (`docs/07`): the model **is**
documented and **does** declare foreign keys. It is the authoritative reference
for our canonical schema — we can port table-for-table instead of inferring.

Also present:

| artifact | what it is |
|---|---|
| `sch.hisinone/experimental/` | same 1458 tables (the working set) |
| `sch.hisinone/extensions/` (29) | customer/extension tables |
| `sch.hisinone/audit/` (11) | the audit/history model (`audit_object`, `audit_link`, `audit_activity`, …) |
| `view.hisinone/stable/*.view` (≈40) | the app's named **joins/denormalisations** in full SQL |
| `scr.hisinone/stable/script.xml` (1.2 MB) | the script/action catalogue (migrations, jobs) |
| `hotfix1006/…/hisDefault/keytables.xml` (82,619 lines) | the **enumerated value catalogue**: 3,532 key tables with their values |

## 2. Domain map (prefix → table count, of 1458)

```
k 352   request 63   einvoice 31   unit 28   person 25   research 21
application 16   allocation 15   doctoral 15   astat 14   course 12
external 12   job 12   project 12   budget 11   overhead 11   publication 11
dynamic 10   employment 10   student 10   duplicates 9   payment 9
personnel 9   portal 9   quota 9   taa 9   cost 8   enrollment 8
examination 8   stu 8   study 8   alumni 7   degree 7   document 7
room 7   chipcard 6   habilitation 6   imputed 6   role 6 …
```

`k_*` (352) are the lookup/reference tables — i.e. the enumerated value sets the
app joins against. The `k_` prefixes above (`stu`, `examination`, `enrollment`,
`course`, `application`, `doctoral`, `research`, `publication`) are
**Campus-Management**; `budget`, `allocation`, `overhead`, `cost`, `payment`,
`personnel`, `einvoice`, `purchase` are **ERP/administration** (campus-erp).

## 3. ERP-relevant tables (149) — the porting targets

Grouped, with the vendor's own comment (translated where useful):

**Cash & bank (Kasse, our pilot):** `cash_account` (26 cols, 13 FKs),
`cashstatement` (Kassenbuch), `cash_terminal`, `bankfile`, `bankstatement`,
`bankstatementline` (28 cols), `banktransaction`, `bank`, `bank_account`.

**Payment traffic:** `payment_order`, `payment_file`, `payment_file_transfer_protocol`,
`payment_element`, `payment_line`, `payment_transaction`, `payment_info`,
`payment_assignment`, `payment_selectable_method` (→ `k_payment_method`).

**Budget (Mittelbewirtschaftung):** `budget_account`, `budget_center`,
`budget_center_budget_position`, `budget_center_budget_source`, `budget_position`,
`budget_position_financial_account`, `budget_position_k_item`, `budget_posting`,
`budget_source`, `budget_source_k_chapter_igr`, `budget_import`.

**Cost accounting (KLR):** `cost_center`, `cost_center_clearing`, `cost_unit`,
`cost_unit_clearing`, `cost_unit_cost_center`, `cost_unit_study`,
`cost_type_bridge` (links FiBu accounts to cost types), `cost_unit_param_group`.

**Overhead / internal pricing:** `overhead_calc` and `overhead_calc_rate*`
(personnel/material/rent rates, static rates, write-offs), `allocation*`
(15 tables: models, keys, steps, rules, exclusions — the **distribution
calculation engine**).

**Personnel cost:** `personnel_cost`, `personnel_cost_*` (bonus, config,
posting, single amounts, financing) — the staff-cost projection model.

**Projects & third-party funds:** `project` (27 cols, 7 FKs), `project_person`,
`project_share`, `project_snapshot*`, `project_subject_area`, `project_orgunit`.

**Procurement:** `purchase_order`, `purchase_order_line`, `purchase_invoice`.

**Assets:** `asset_card`, `asset_component` (Anlagenbuchhaltung).

**Accounting core:** `financial_account` (chart of accounts), `accounting_unit`
(Buchungskreis), `accounting_record`, `accountingflag_*`.

**Personnel master:** `employee`, `employee_info` (33 cols), `employee_astat`,
`employee_taa_config`; `absence`, `absence_basic_data`, `absence_type`.

**E-invoicing (31 tables):** `einvoice*` — see §4.

## 4. E-invoicing is already EN16931-shaped — reuse the whole model

`einvoice_*` maps one-to-one onto the **EN16931 / ZUGFeRD business groups**:
`einvoice_metadata` (entry point), `einvoice_process_control` (BG-2),
`einvoice_preceding_invoice_reference` (BG-3), `einvoice_seller` (BG-4),
`einvoice_buyer` (BG-7), `einvoice_line` (BG-25), `einvoice_vat_breakdown`
(BG-23), `einvoice_document_total` (BG-22), `einvoice_payment_instructions`
(BG-16), `einvoice_credit_transfer` (BG-17), etc.

The code lists it depends on are standard (from `keytables.xml`):

| key table | standard |
|---|---|
| `k_einvoice_type` | UNTDID **1001** Document Type Code |
| `k_einvoice_payment_means_type` | UNTDID **4461** Payment Means |
| `k_einvoice_vat_category` | UNTDID **5305** VAT Category |
| `k_einvoice_unit_of_measure` | UNECE Units of Measure |
| `k_einvoice_allowance_reason` / `_charge_reason` | Codeliste **5189** / **7161** |
| `k_einvoice_electronic_address_scheme` | CEF **EAS** |
| `k_einvoice_vat_profile` | maps E-invoice VAT onto the MBS VAT logic |

The `rm.erp.xrechnung.jar` (1,456 classes) implements **both syntaxes**: UBL
(XRechnung) and UN/CEFACT CII (ZUGFeRD) — `rm.erp.xrechnung` +
`un.unece.uncefact.data.standard.crossindustryinvoice._100`. German public bodies
must accept XRechnung, so this is a **must-have** capability with a ready
reference implementation and schema.

## 5. Enumerated value catalogue — 3,532 key tables with values

`hotfix1006/WEB-INF/conf/hisinone/confPackages/hisDefault/keytables.xml` holds
every coded value set with short/long text, language, sort order and a stable
`HiskeyId`. Notable sizes (from the header): `StatusValue` 218,
`PortalMessageText` 328, `PortalMessageCategory` 177, `FinancialDocumentTypeValue`
46, `LanguageValue` 183, `PeriodUsageValue` 33, `CashExportformatValue` 14
(e.g. "SEPA 3.5"), `FinancialAccountCategoryValue` 19, `PurposeValue` 31.

This is the seed data for our lookup tables — it can be loaded directly instead
of hand-maintaining catalogues.

## 6. The app's joins and scripts

- `view.hisinone/stable/*.view` — named views in full SQL, e.g.
  `person_complete_name` (the title/degree/salutation composition logic incl.
  female/diverse variants and i18n via `foreigntext`), `orgunits_flat`,
  `stu_join_studierendenstammdaten`. These are the semantics behind the model.
- `scr.hisinone/stable/script.xml` — the migration/job action catalogue.
- The legacy `_sbid` views (`docs/07`) are the same idea on the legacy side.

## 7. Legacy reference data (already in `docs/07`)

Reusable, populated, real: `kbs.blz` (3,732 bank directory rows),
`kbs.bank_sctsdd` (4,912 **SEPA scheme participation per BIC**), the BIC
directories, `mbs.fibu_konten` (1,001 chart-of-accounts rows),
`mbs.fibu_kontenstand` (2,001 monthly balances), `kbs.basiszins` (45 base
interest rates since 2002), `kbs.mwst_art` (32 VAT types).

## 8. Source-side capabilities (business logic)

From `docs/05` plus this pass:

- **Payment traffic** (`rm.fia.payments.datex`, 1,651 sources): ISO 20022
  pain.001.001.03/**09 (GBIC 4/5)**, pain.008.001.02/**.08**, camt.052/053/054
  .02/**.08**, MT940, DTAUS, CSV (LKB), MACH, SAP, EBICS — with the shipped XSDs.
- **E-invoicing**: `rm.erp.xrechnung.jar` (UBL + CII, KoSIT validation helpers).
- **Legacy bridge**: `rm.erp.legacy.jar` (81 classes,
  `de.his.legacy.persistence.dao.impl.rm.erp.*`) issues the generated
  `<table><suffix>` DB calls (`docs/07` §2) — the place to read a legacy
  read/write path exactly.
- **Backend**: `rm.erp.backend.jar` (1,593 classes) — procurement, invoicing,
  ServiceBW.

## 9. What to use, in priority order

1. **Port the canonical schema from `sch.hisinone/stable/*.tab|.fk`** — it has the
   comments and the FKs the legacy dump lacked; table-for-table, ERP domains
   first. (`docs/06` already did the cash/bank cross-walk.)
2. **Seed the lookup tables from `keytables.xml`** — replaces hand-maintained
   catalogues (VAT types, payment methods, document types, currencies…).
3. **Build XRechnung/ZUGFeRD e-invoicing** on the `einvoice_*` model + the
   standard code lists — mandatory for German public bodies, and the reference
   implementation ships here.
4. **Load the reference data** from the dump (bank directory, SEPA scheme,
   chart of accounts, base interest rates).
5. **Model budget / cost accounting / overhead / allocation** from the vendor
   tables when the pilot moves past Kasse (Phase 2+).
6. **Adopt the audit model** (`sch.hisinone/audit/`, 11 tables) and the `_sbid`
   authorisation semantics.

## 10. Caveats

- The archives are **proprietary**; nothing is copied into this repo — this doc
  and the code we write are original, referencing paths only.
- The vendor model is large (1458 tables). campus-erp is a *better*, not a
  *bigger*, implementation: reuse the model where it is sound, and keep our
  stricter constraints (integer cents, declared FKs, enum checks) per the ADRs.
