# 05 — Productive-source inventory (the reference implementation)

What the three archives handed over in `~/Downloads` actually contain, how they
were inspected, and what they unlock for this project.

**Provenance & handling.** The archives are the **productive the reference implementation** build
(release `2026.06`, build `2026.06.13034`, HISINONE hotfix 1, QIS hotfix 1). They
are **proprietary and are never committed** (AGENTS.md rule 1/2). They were
extracted outside the repository to `the reference source/` and are cited below by
their in-archive path. Everything here is *measured* on those files unless marked
*(inferred)*.

## 1. What the archives are

| Archive (in `~/Downloads`) | Size | Content |
|---|---|---|
| `H1-CS-CM-RM.zip` | 4.2 GB | Exploded WAR of the whole product (`webapps/the reference application`, `ROOT`, `the legacy front-end`); all module jars, all Java sources (`WEB-INF/src/java`, 21 138 files), config, UI pages |
| `hotfix_…2026-10-1…AFCTS.zip` | 327 MB | The RM/ERP extension jars + `WEB-INF/conf` (key tables, roles, i18n) |
| `hotfix_…2026-10-6…se4F1.zip` | 328 MB | Same, one week newer (used as the reference here) |

`H1-CS-CM-RM` = the product bundle **CS** (Campus) · **CM** (Campus Management) ·
**RM** (Ressourcenmanagement = the ERP). The two hotfixes are ERP-only deltas.

## 2. Module catalogue

`conf/modules.xml` declares **341 active servlet modules** with their rights
(`authorities="RIGHT_…"`). The extension jars group into families:

| Family | Meaning | Examples |
|---|---|---|
| `cm.*` | Campus Management | `cm.app.*`, `cm.exa.*` (exams), `cm.stu.*` (students, doccand, epay) |
| `cs.*` | Core system | `cs.sys.*` (base, audit, portal, rest, dbschema), `cs.psv.*` (persons), `cs.bia.*` |
| `fs.*` | Research / third-party funds | `fs.doc.*`, `fs.res.*` |
| `the ERP *` | **ERP** (the module this project targets) | `the ERP backend`, `.frontend`, `.legacy`, `.xrechnung`, `.servicebw.sgw` |
| the finance packages | **Finanzabteilung** (finance) | the finance export mapping |

## 3. The vendor data model — authoritative, with FKs and comments

`the reference schema jar` (296 MB) carries the full schema in
`META-INF/resources/conf/dbconv/conf/sch/`:

| Path | What |
|---|---|
| `the reference DDL/experimental/dbmodel.ddl` | **1 486 `CREATE TABLE`** + `COMMENT ON TABLE/COLUMN` (German), PostgreSQL |
| `the reference DDL/experimental/dbmodel_fixed.ddl` | Same, framework-corrected (`obj_guid`, `lock_version` added) |
| `the reference tables/*.tab` | 1 458 tables, DDL with **inline `{German comments}`** per column |
| `the reference tables/*.fk` | **1 286 declared `FOREIGN KEY` constraints** |
| `the reference tables/*.idx` | 1 447 indexes |
| `the reference views/*.view` | 18 views |
| `scr.the reference source/stable/script.xml` | 1.25 MB schema-conversion script |

This is the decisive contrast with the legacy dump we reverse-engineered
(`docs/02`): the dump has **1 536 tables, 0 FKs, no comments**; the productive
schema has **declared FKs and full column comments**. It is the same domain
(university ERP/campus) modelled cleanly — the vendor's own successor to the
legacy system.

## 4. Configuration catalogues

| File (`WEB-INF/conf/…`) | Content |
|---|---|
| `modules.xml` | 341 modules + rights |
| `the reference source/confPackages/default/keytables.xml` | **3 532 key-table records** (`Schlüsseltabellen`) |
| `the reference source/confPackages/default/keytablesOwnedByHIS.xml` | 2.3 MB, vendor-owned key tables |
| `the reference source/confPackages/default/role.xml` | **1 762 `RightsDefinition`, 7 279 `RightsInRole`, 126 org roles** — the permission model |
| `the reference source/confPackages/default/job.xml` | 14 MB job catalogue |
| `the reference source/confPackages/default/the reporting centre.xml` | 1.6 MB report catalogue |
| `languages/default/i18n.rm.erp_{de,en}.txt` | ERP field labels (semantics) |
| `the import configuration file` | 1.5 MB import configuration |

## 5. The ERP (RM) source

`the ERP backend.jar` contains **1 481 Java sources** under `de/his/`:

- `de/his/erp/financialmanagement/einvoice` — e-invoicing (XRechnung/ZUGFeRD, KoSIT validation)
- `de/his/erp/rechnung` — invoicing (`ubl`, `cii`, `servicebw`, `servicesn`)
- `de/his/erp/procurement` — requisition, order, online shop
- `de/his/erp/konsens/kmv` — 181 sources (KMV budget/funds handling)
- `de/his/erp/travelmanagement`, `absencemanagement`, `timemanagement`, `jobapplication`, `inventory`, `mbsprotocol`, `processcancellation`
- `de/his/appserver/{model,dto,service,persistence,reporting}/rm/**` — Hibernate entities and DAOs

`the ERP xrechnung.jar` carries the XRechnung/ZUGFeRD rendering and KoSIT
runtime validation; `the ERP legacy.jar`, `the ERP base.jar`,
the ERP service jar, the finance export-mapping jar are the seams.

## 6. Payment traffic — the interface layer (`the reference parser`)

**1 651 sources.** The vendor's whole bank-file layer:

| Format | Where |
|---|---|
| SEPA credit transfer | `…/the reference/sepa/pain001V00103`, `pain001V00109`, `pain001V00109GBIC5` |
| SEPA direct debit | `…/the reference/sepa/pain008V00102`, `pain008V00108`, `pain008V00108GBIC5`, `mandate` |
| Bank statements | `…/the reference/mt940`, `…/the reference/sepa/camt053v02/v08`, `camt054v02/v08` |
| Legacy / other | `dtaus`, `csv` (LKB), `mach`, `sap`, `sapFRMA`, `mbs`, `kabu`, `mapping` |

**ISO 20022 XSDs shipped** (authoritative, no guessing needed):
`pain.001.001.03`, `pain.001.001.09` (GBIC 4/5), `pain.008.001.02`,
`pain.008.001.08` (GBIC 4/5), `camt.052/053/054.001.02` and `.001.08`.

## 7. What this enables

1. **Unblocks S3 (SEPA export) and S4 (statement import).** The XSDs plus the
   vendor's field mapping replace the "golden files" gap for the *format*; only
   a per-bank sample remains.
2. **Reference model for the canonical schema.** The productive schema's FKs +
   comments are an evidence base to validate our own design (`docs/adr/0001`);
   the concrete comparison is in `docs/06-vendor-model-crosswalk.md`.
3. **Business-rule reference for later phases** (procurement, invoicing /
   XRechnung, travel — i.e. Phase 4).
4. **Answers to the open questions** (below).

## 8. Impact on `docs/open-questions.md`

- **Q1 (which product)** — the productive target is **the reference implementation** (unified schema,
  English table names); the dump is the **legacy the legacy university ERP** (module schemas
  `sva4/mbs/cob/kbs`, German abbreviations). Two generations of the same vendor
  line; they share no tables. *Evidence: `the reference DDL/experimental/dbmodel.ddl`
  vs the generated table list (`scripts/analyze.sh`).*
- **Q5 (column semantics)** — available for the **productive** product
  (`COMMENT ON`, `{…}` in `.tab`, i18n); the legacy dump still has none.
- **Q6 (authoritative tables)** — `the reference tables` is the vendor's live set.
- **Q7 (relationships)** — 1 286 declared FKs in `*.fk` (productive); the legacy
  dump stays 0-FK.
- **Q8 (interface contracts)** — SEPA/CAMT/MT940/XRechnung now available as XSDs
  and reference code.

## 9. Reproducing the extraction

```bash
mkdir -p the reference source && cd the reference source
unzip -q ~/Downloads/hotfix_…2026-10-6….zip -d hotfix1006      # ERP jars + conf
unzip -q ~/Downloads/H1-CS-CM-RM.zip '…/WEB-INF/src/java/*' -d h1   # full Java source
unzip -q … the reference schema jar '…/conf/dbconv/conf/sch/the reference DDL/*' …
```
