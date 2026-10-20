# C4 — Level 2: Container

The application tier is **inferred** from the database structure (the dump is a
database only). The database container is **measured**.

## Diagram

```mermaid
C4Container
    title Container — Hochschul-ERP

    Person(user, "Sachbearbeiter:in", "HR / finance / cashier staff")

    Container(web, "Web-Oberfläche / Portal", "Browser app", "Role-specific UI; metadata-driven forms from db_tabellen/db_spalten (inferred)")
    Container(app_pers, "HISSVA application", "App server", "Personal- und Stellenverwaltung (inferred)")
    Container(app_fin, "HISMBS / HISFSV application", "App server", "Finanz- und Sachmittelverfahren (inferred)")
    Container(app_cob, "HISCOB application", "App server", "Kosten- und Leistungsrechnung (inferred)")
    Container(app_kbs, "HISKBS application", "App server", "Kassenbuchungssystem (inferred)")
    Container(workflow, "Workflow- & Rechte-Engine", "In-app framework", "sv_* / wof: Vorgänge, Aufgaben, Rechte, Kataloge (inferred)")
    ContainerDb(db, "PostgreSQL database", "PostgreSQL 17", "One database, four module schemas + framework copies. 1,536 tables, 0 FKs")
    Container(batch, "Batch / Schnittstellen", "File & DB jobs", "SEPA, MT940, LBV, ELSTER, statistics exports (imp_*/out_*)")
    Container(dms, "Dokumentenablage", "DB + filesystem/DMS", "dokumente tables; DB, filesystem, or external DMS")

    Rel(user, web, "nutzt", "HTTPS")
    Rel(web, app_pers, " ")
    Rel(web, app_fin, " ")
    Rel(web, app_cob, " ")
    Rel(web, app_kbs, " ")
    Rel(app_pers, workflow, " ")
    Rel(app_fin, workflow, " ")
    Rel(app_cob, workflow, " ")
    Rel(app_kbs, workflow, " ")
    Rel(app_pers, db, "JDBC/SQL", "schema sva4")
    Rel(app_fin, db, "JDBC/SQL", "schema mbs")
    Rel(app_cob, db, "JDBC/SQL", "schema cob")
    Rel(app_kbs, db, "JDBC/SQL", "schema kbs")
    Rel(batch, db, "SQL/file")
    Rel(app_pers, dms, " ")
    Rel(app_fin, dms, " ")
```

## Containers

| Container | Evidence / basis |
|---|---|
| Web-Oberfläche / Portal | HIS product material ("webbasiert", "Portal", "responsive") — **[c]** |
| HISSVA app | `sva4.db_version` = `HISSVA-GX` — **[c]** |
| HISMBS / HISFSV app | `mbs.db_version` = `HISMBS-UNIX` / `HISFSV-GX` — **[c]** |
| HISCOB app | `cob.db_version` = `HISCOB-GX` — **[c]** |
| HISKBS app | `kbs.db_version` = `HISKBS` — **[c]** |
| Workflow & Rechte engine | framework tables `sv_*`, `wof` — **[c]** (existence), **[i]** (that it is a shared in-app engine) |
| PostgreSQL database | measured — **[c]** |
| Batch / interfaces | `imp_*`/`out_*`, SEPA/MT940/LBV tables — **[c]** |
| Document storage | `dokumente`, `k_dokumente_*`; HIS material names DB/filesystem/DMS options — **[c]** |

## Container-level observations

1. **One database, four module schemas, no shared schema.** Each module app owns
   its schema and carries its own framework copy (see `02-database-overview.md`
   §2). There is no central master-data schema in the dump.
2. **A separate "core" schema is minimal.** `hisrm` holds only 3 tables
   (`_customize_dumping`, `illchars_*_archived`) — it is not a shared business
   core, despite the database name.
3. **Integration is file/batch-centric** (`imp_*` staging tables, `out_*`
   exports, MT940/SEPA files) rather than a service layer, as far as the schema
   shows.
4. **The metadata layer is a container of its own in effect:** `db_tabellen`,
   `db_spalten`, `db_graph_knoten/kanten/joins` drive UI navigation dynamically —
   a significant design feature and a reverse-engineering target.
