# 14 — The rest of the bundle: the legacy front-end, the WDK SOAP surface and the shipped connectors

`docs/05` and `docs/08` inventoried the parts of the 4.2 GB `H1-CS-CM-RM.zip` that
matter for the data model and the ERP code (`webapps/qisserver/WEB-INF`,
`cs.sys.dbschema.hisinone.jar`, the hotfixes). This note closes the inventory:
the **rest of the bundle** — the legacy ERP's own web front-end (`superx`), the
whole **SOAP interface surface** (the `WDK` SoapUI project) and the shipped
**connectors** (`sonst/`). Everything here is *measured* on the extracted files
unless marked *(inferred)*. As everywhere in this repo, the archives are
proprietary and are never committed (AGENTS.md 1/2); they are cited by in-archive
path.

## 1. Bundle layout (the 4.2 GB `H1-CS-CM-RM.zip`)

```
2026-08-23/
  webapps/qisserver   127 750 entries  HISinOne app (docs/05) — the product
  webapps/superx       92 516 entries  the LEGACY ERP front-end + tooling  (new here)
  webapps/ROOT          2 733 entries  shared ROOT webapp (HISinOne/, QIS/, index)
  sonst/bsos-connector                  chip-card / cash-terminal bridge     (new here)
  sonst/hs2kfw                          KfW student-loan SOAP client         (new here)
  sonst/soapui                          WDK-HISinOne-soapui-project.xml      (new here)
```

`superx` alone is bigger than everything analysed so far; it was skipped earlier
because it is not (table) source. It is, however, the *other half of the product*:
HISinOne is the new campus app, `superx` is the legacy ERP's own UI and tooling.

## 2. `superx` — the legacy ERP's front-end and tooling

| subtree | entries | what it is |
|---|---|---|
| `edit/` | 493 | the **form-based admin modules** (see below) |
| `xml/` | 3 913 | form/report definitions |
| `WEB-INF/` | 14 899 | server-side includes/classes |
| `dbt/` | 71 550 | a bundled **"DB-Tools" Python environment** + `dbt/projects`, `scripts`, `requirements.txt`, `create_dbt_env_{linux,windows}` — the vendor's schema/DDL deployment tool |
| `saiku/` | 442 | **Saiku** — an open-source OLAP/BI front-end (the KLR/controlling analytics) |
| `jasper/` | 4 | **JasperReports** — the print/report engine (Bescheide, certificates, lists) |
| `dbformslib/` | 6 | the form library used by `edit/` |

`edit/` module codes line up with the **legacy dump schemas** (`docs/03`):

```
cob  fin  sva  sos  astat  bau  biad  erfolg  gang  ivs  kenn  kern  lm  man  prom  res  zul
```

`cob` = KLR, `fin`/`mbs` = Finanzen, `sva` = Personal, `sos` = ? , `astat` =
statistics, `bau` = buildings/property (GLM), `kenn`/`kern` = core/registration,
`prom`/`zul` = promotions/admission, `res` = research, `ivs` = ? , `gang` =
workflow (`Vorgang`), `lm` = ? , `man` = manual/master data. This is the concrete
**module → screen map** for the processes we must reach parity with (`FR-*`), and
the legacy counterpart of the Jasper/Saiku/`dbt` trio.

## 3. The WDK SOAP surface — 79 services, 638 operations

`sonst/soapui/WDK-HISinOne-soapui-project.xml` (3.2 MB) is the vendor's SoapUI
project for the **WDK** (the HISinOne web-service layer). It declares:

- **81 service definitions / 79 bindings**, all under `http://www.his.de/ws/<Service>`;
- **638 distinct `soapAction`s** (operations).

The ERP/administration-relevant services (these are the interfaces a replacement
must reach parity with — `IR-7`):

| service | representative operations | domain |
|---|---|---|
| `PaymentExportService` | `createOutgoingPayments`, `createDirectDebitPayments`, `createOutgoingReturnRemittance`, `readOutgoingPayments`, `updateStatusOfPayment`, `updateConfirmReceiptOfPayments` | **SEPA / payment traffic** (the SOAP face of `rm.fia.payments.datex`) |
| `CardTerminalService` | `readActiveInvoicesByPersonId`, `readFee`, `saveCashStatementByInvoices`, `saveCashStatementByPersonId`, `reregisterStudentByPersonId` | **cash terminal / Kasse** |
| `ChipcardService` | `findChipcard`, `readChipcard`, `saveChipcards`, `createSubstituteChipcard` | campus card |
| `AccountService` | `createAccount`, `changePasswordForAccount`, `searchAccountForPerson` | user accounts (IdM) |
| `RightsAndRolesService` | `readActiveRightsForRole`, `findRoles60`, `createOrgroleForPerson` | **permission model** (`FR-X1`) |
| `ProjectService` | `readProject`, `findByOrgunit` | third-party funds (Drittmittel) |
| `EmploymentsService` | `saveEmployment`, `loadEmploymentsOfPerson` | HR link |
| `FacilityService`, `UnitService`, `OrgunitService` | org units / facilities | org structure |

Campus services (Person, Address, Applicant, Application, Course, Exam,
AdmissionPackage, Alumni, DoctoralCandidate, Disenrollment, LeaveOfAbsence,
DegreeProgramProgress, …) are out of scope for the ERP but show the SOAP layer is
**one uniform surface** for the whole product — and that the ERP functions
(payment, card terminal, accounts, rights) are exposed through it.

One **customer-specific** service is embedded: `UniBielefeldIdmConnectorService`
(`targetNamespace="https://campus.uni-bielefeld.de/ws/..."`). Evidence that a
university's identity management is wired in as a connector, not hard-coded.

## 4. The shipped connectors (`sonst/`)

The bundle also ships two working **external-interface adapters** — the concrete
counterparts of the `connectors` role (ADR-0005):

### 4.1 `bsos-connector` — chip-card / cash-terminal bridge

A small Python bridge (`bsosclient.py`, `bsosserver.py`, `helper.py`, `conf.ini`,
`README.txt`, references the HIS wiki page *BSOS-Python-Connector*):

- `bsosserver.py` is a **TCP server** that receives a customer command string and
  `os.spawnl`s the Windows program **`bsos.exe`** (`[server]bsosexe`), then streams
  the produced return file back to the client, terminated by `endtag=BsOsEnD`.
- `bsosclient.py` sends a command (`rueckmeldung` / `proberueckmeldung` /
  `buchung` / `chippers`) plus a **Matrikelnummer** and writes the return file
  (`Rue_<mtknr>.log`, `Buch_<mtknr>.log`, `Chip_<mtknr>.log`) to disk.
- `conf.ini` warns explicitly: *"Da Paßworte im Klartext übermittelt werden, sollte
  die Verbindung gesichert sein"* (passwords go over the wire in clear text; use
  stunnel). *(inferred)* the design assumption is a trusted campus LAN.

This is the **cash-terminal / chip-card interface** — the `CardTerminalService`
above is its SOAP-facing sibling.

### 4.2 `hs2kfw` — KfW student-loan SOAP client

`hs2kfw/bin/hs2kfw_{ux,win}.cmd` launch
`de.his.xml.soap.client.KfwAxis2TestClient` against
`https://onlinekreditportal.kfw.de:10443/kfwservices/services/SBwsService`
(webapp `studarlehennds`). A **SOAP/Axis2 client for the KfW student-loan portal**
— an external financial interface (IR-class) with a shipped client.

### 4.3 The payment/bank layer (recap)

`rm.fia.payments.datex` (`docs/05` §6, 1 651 sources) plus the ISO 20022 XSDs is
the bank side; `PaymentExportService` is its SOAP face; `bsos`/`kfw` are the
device/KfW sides. Together they are the vendor's **complete external-interface
inventory**.

## 5. What this changes for campus-erp

1. **Interface/connector catalogue (`ADR-0005`).** The bundle lists the real
   external interfaces a drop-in replacement must implement: bank
   (SEPA/datex + SOAP `PaymentExportService`), cash/chip terminal (`bsos` +
   `CardTerminalService`), KfW (`hs2kfw`), IdM (`UniBielefeldIdmConnector`),
   ELSTER/LBV/statistics (`IR-1..IR-4`). They belong in one registry, each with a
   versioned format contract and a golden file (`requirements.md` §5).
2. **Reporting/BI (`FR-X2`).** `superx` shows the legacy reporting stack
   (**JasperReports** for documents, **Saiku OLAP** for controlling analytics) —
   the parity targets for our reporting centre and KLR evaluations (`FR-C6`).
3. **Module → screen map.** `superx/edit/*` is the concrete process inventory per
   module; it grounds the `FR-*` parity matrix and the ETL scope per phase.
4. **The legacy schema tool.** `superx/dbt` (the vendor's own schema/DDL deployer
   with Python envs for 3.10–3.13) confirms how the legacy DDL was applied — useful
   context for the migration tooling (`MR-1`), not code we reuse.
5. **SOAP parity is real scope.** 638 operations across 79 services is a large
   interface surface; the ERP-relevant subset (payment, card terminal, accounts,
   rights, project) is the part `IR-7` must reproduce.

## 6. Reproducing the extraction

```bash
cd ~/Work/hisinone/src/h1/2026-08-23
unzip -q ~/Downloads/H1-CS-CM-RM.zip \
  '2026-08-23/sonst/*' '2026-08-23/webapps/ROOT/*' -d .
# superx is large; list it without extracting:
unzip -l ~/Downloads/H1-CS-CM-RM.zip | grep 'webapps/superx/'
```
