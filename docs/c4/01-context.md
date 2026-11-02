# C4 — Level 1: System Context

Scope: the **legacy university ERP** family the sample dataset belongs to.
Everything here is reconstructed from that dataset plus public product material; the application tier is **inferred** from the database, since only the
database was provided. Inference is marked *(inferred)*.

## Diagram

```mermaid
C4Context
    title System Context — the legacy system

    Person(sachb_pers, "Personalsachbearbeiter", "HR administration staff")
    Person(sachb_fin, "Finanzsachbearbeiter", "Finance administration staff")
    Person(kasse, "Kassenverwalter", "Cashier")
    Person(leitung, "Hochschulleitung", "Management / controlling")
    Person(pruefer, "Revision / Prüfung", "Audit / internal revision")

    System(erp, "the legacy university ERP", "Personal, Finanzen, Sachmittel, Kasse, Kosten-/Leistungsrechnung (modules the HR module, the finance module/the finance module, the cost-accounting module, the cash-book module)")

    System_Ext(the reference source, "the reference implementation", "Campus management (APP/STU/EXA/DOC)")
    System_Ext(lbv, "LBV / Bezügestelle", "State payroll office (Besoldung/Bezüge)")
    System_Ext(bank, "Banken / SEPA", "Bank & payment traffic (SEPA, MT940)")
    System_Ext(finanzamt, "Finanzamt / ELSTER", "Tax office / e-tax filing")
    System_Ext(dms, "DMS", "Document management system")
    System_Ext(statistik, "Statistik / Landesamt", "Statistics offices")
    System_Ext(qis, "QIS / SAP HCM", "External campus & HR systems")
    System_Ext(bi, "Business Intelligence", "BI/reporting / reporting")
    System_Ext(idp, "IdP (Shibboleth/OIDC)", "Identity provider (IR-5)")
    System_Ext(kfw, "KfW Studentendarlehen", "Loan SOAP portal (the student-loan client, docs/14 §4.2)")
    System_Ext(the card terminal, "Chipkarte / Kassenautomat", "BSOS terminal (the card-terminal connector, docs/14 §4.1)")
    System_Ext(hss, "hochschulstart.de / DoSV", "Admission portal (IR-8)")

    Rel(sachb_pers, erp, "verwaltet Personal, Stellen, Reisen")
    Rel(sachb_fin, erp, "verwaltet Haushalt, Rechnungen, Beschaffung")
    Rel(kasse, erp, "führt Kassenbuch, Zahlungsverkehr")
    Rel(leitung, erp, "Controlling, Auswertungen")
    Rel(pruefer, erp, "Revision, Protokolle")

    Rel(erp, the reference source, "Stammdaten / Kostenstellen / Studierende", "Schnittstelle (inferred)")
    Rel(erp, lbv, "Bezüge-/Besoldungsmeldungen", "File/Datenlieferung (bw_lbv_*, nrw_*)")
    Rel(erp, bank, "SEPA-Zahlungen, MT940-Auszüge", "Datei/Bank (bic_sepa_*, mt940)")
    Rel(erp, finanzamt, "Umsatzsteuer-Meldungen", "ELSTER (elster_webserver)")
    Rel(erp, dms, "Dokumente", "DMS-Anbindung (dokumente, DMS)")
    Rel(erp, statistik, "Personal-/Finanzstatistik", "Export (out_*)")
    Rel(erp, qis, "Personendaten-Transfer", "qis_sva_personendaten_transfer")
    Rel(erp, bi, "Auswertungen/Kennzahlen", "Datenbank-Views")
    Rel(erp, idp, "Login / Rechte", "OIDC/SAML (IR-5)")
    Rel(erp, kfw, "Studienkredit-Anträge", "SOAP SBwsService (docs/14 §4.2)")
    Rel(erp, the card terminal, "Karten-/Kassenautomat", "TCP-Bridge (BSOS, docs/14 §4.1)")
    Rel(erp, hss, "Bewerbung / DoSV", "IR-8")
```

> For the **replacement's** external systems and each interface's
> implemented/planned status, see `05-external-systems.md`.

## Actors

| Actor | Uses |
|---|---|
| Personalsachbearbeiter | `sva4` — personnel, positions, travel, payroll interfaces |
| Finanzsachbearbeiter | `mbs` — budget, invoices, procurement, property, inventory |
| Kassenverwalter | `kbs` — cash book, payment traffic, dunning |
| Controlling / Leitung | `cob` + reports across modules |
| Revision / Prüfung | audit/history data (`rev_*`, `*prot`, `__account_history`) |

## External systems (from evidence in the schema)

| External system | Evidence |
|---|---|
| LBV / payroll office | `sva4.bw_lbv_*`, `nrw_lbv_*`, `lbv_*` tables |
| Bank / SEPA | `mbs.bic_sepa_credit_transfer`, `bic_sepa_direct_debit`, `direct_debit_mandate`; `kbs.mt940`, `zp_bankdaten` |
| Finanzamt / ELSTER | `mbs.elster_webserver` |
| QIS | `sva4.qis_*`, `qis_sva_personendaten_transfer` |
| SAP HCM | `sva4.h1_sva_transfer`, `h1_sva_attribute_mapping` |
| DMS | `dokumente`, `__dokumente*`, `k_dokumente_*` across schemas |
| Statistics offices | `sva4.k_referenz_personalstatistik`, `out_*` exports |
| the reference implementation campus | inferred; not present as tables in this dump |
| Identity provider (Shibboleth/OIDC) | `docs/14` (bundle ships an IdM connector: `UniBielefeldIdmConnectorService`) |
| KfW student loan | `the student-loan client` — a shipped Axis2 SOAP client for the KfW portal (`docs/14` §4.2) |
| Chip-card / cash terminal (BSOS) | `the card-terminal connector` (TCP bridge to `the card terminal.exe`, `docs/14` §4.1) |
| hochschulstart.de / DoSV | requirement IR-8 (admission); no tables in this dump |
| Bank (EBICS channel) | `docs/14` §4.3 — the SEPA file layer plus an EBICS transport, beyond the file tables above |

## Key open point

Whether the project's "system of interest" is this **the legacy university ERP** or the
separate **the reference implementation campus** product is unresolved (see `open-questions.md` Q1).
This context diagram documents the ERP, which is what the dump contains.
