# C4 — Level 1: System Context

Scope: the **Hochschul-ERP** system (the product family the seed dump belongs
to). Everything here is reconstructed from the dump plus HIS's public product
material; the application tier is **inferred** from the database, since only the
database was provided. Inference is marked *(inferred)*.

## Diagram

```mermaid
C4Context
    title System Context — Hochschul-ERP (HIS eG)

    Person(sachb_pers, "Personalsachbearbeiter", "HR administration staff")
    Person(sachb_fin, "Finanzsachbearbeiter", "Finance administration staff")
    Person(kasse, "Kassenverwalter", "Cashier")
    Person(leitung, "Hochschulleitung", "Management / controlling")
    Person(pruefer, "Revision / Prüfung", "Audit / internal revision")

    System(erp, "Hochschul-ERP", "Personal, Finanzen, Sachmittel, Kasse, Kosten-/Leistungsrechnung (modules HISSVA, HISMBS/HISFSV, HISCOB, HISKBS)")

    System_Ext(hisinone, "HISinOne", "Campus management (APP/STU/EXA/DOC)")
    System_Ext(lbv, "LBV / Bezügestelle", "State payroll office (Besoldung/Bezüge)")
    System_Ext(bank, "Banken / SEPA", "Bank & payment traffic (SEPA, MT940)")
    System_Ext(finanzamt, "Finanzamt / ELSTER", "Tax office / e-tax filing")
    System_Ext(dms, "DMS", "Document management system")
    System_Ext(statistik, "Statistik / Landesamt", "Statistics offices")
    System_Ext(qis, "QIS / SAP HCM", "External campus & HR systems")
    System_Ext(bi, "Business Intelligence", "HISinOne-BI / reporting")

    Rel(sachb_pers, erp, "verwaltet Personal, Stellen, Reisen")
    Rel(sachb_fin, erp, "verwaltet Haushalt, Rechnungen, Beschaffung")
    Rel(kasse, erp, "führt Kassenbuch, Zahlungsverkehr")
    Rel(leitung, erp, "Controlling, Auswertungen")
    Rel(pruefer, erp, "Revision, Protokolle")

    Rel(erp, hisinone, "Stammdaten / Kostenstellen / Studierende", "Schnittstelle (inferred)")
    Rel(erp, lbv, "Bezüge-/Besoldungsmeldungen", "File/Datenlieferung (bw_lbv_*, nrw_*)")
    Rel(erp, bank, "SEPA-Zahlungen, MT940-Auszüge", "Datei/Bank (bic_sepa_*, mt940)")
    Rel(erp, finanzamt, "Umsatzsteuer-Meldungen", "ELSTER (elster_webserver)")
    Rel(erp, dms, "Dokumente", "DMS-Anbindung (dokumente, DMS)")
    Rel(erp, statistik, "Personal-/Finanzstatistik", "Export (out_*)")
    Rel(erp, qis, "Personendaten-Transfer", "qis_sva_personendaten_transfer")
    Rel(erp, bi, "Auswertungen/Kennzahlen", "Datenbank-Views")
```

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
| HISinOne campus | inferred; not present as tables in this dump |

## Key open point

Whether the project's "system of interest" is this **Hochschul-ERP** or the
separate **HISinOne campus** product is unresolved (see `open-questions.md` Q1).
This context diagram documents the ERP, which is what the dump contains.
