# HIS product & module catalogue

Sources: his.de product pages (`/hisinone`, `/loesungen`, `/hochschul-erp`,
`/dienstleistungen`, `/hisinone/studierende`, `/hisinone/pruefungen-und-veranstaltungen`),
and the module identifiers recorded in the dump's own `db_version` tables.

HIS eG (Hannover) ships several product lines. Two matter here:

1. **HISinOne** — campus management (student lifecycle).
2. **Hochschul-ERP** — "Ressourcenmanagement": finance, personnel, resources.

They are separate products that share technology and integrate, but they are
**not** the same database. The seed dump is a **Hochschul-ERP** database.

---

## 1. HISinOne — Campusmanagement (student lifecycle)

Organised by lifecycle stage; each stage is a module with a short code.

| Code | Module | Lifecycle stage / scope |
|---|---|---|
| `HISinOne-APP` | Bewerbung und Zulassung (Application & Admission) | Information, Online-Bewerbung, Zulassung |
| `HISinOne-STU` | Studierendenmanagement (Student management) | Immatrikulation, Rückmeldung, Beurlaubung, Exmatrikulation, Gebühren, Statistik, Krankenversicherung/SMV |
| `HISinOne-EXA` | Prüfungen & Veranstaltungen (Exams & courses) | Curriculum Designer, Semester-/Veranstaltungsplanung, Prüfungen, Leistungsverbuchung, Praktika |
| `HISinOne-DOC` | Doktorandenmanagement (Doctoral management) | Registrierung → Promotionsverfahren, Promovierendenstatistik |
| — | Alumni | Zentrales Portal für alle Zielgruppen |
| — | Forschungsmanagement (Research management) | KDSF-basierte Forschungsmetadaten |
| — | Portal | Central role-specific entry point |
| — | Business Intelligence | Kennzahlen, Management-Cockpit |
| — | HIS-SaaS | Hosting/operations offering |

Cross-cutting platform features (apply to all modules): Dokumentenmanagement,
Leistungsstarke Suche, Identitätsmanagement & Verzeichnisdienst, Berichtscenter,
Barrierefreiheit (BITV 2.0 / WCAG), DSGVO-Unterstützung.

> The `APP/STU/EXA/DOC` codes are confirmed by HIS's own marketing copy
> (e.g. "Mit HISinOne-EXA realisieren wir … Veranstaltungen und Prüfungen").
> Alumni / Forschungsmanagement codes are not stated publicly and are left blank
> rather than guessed.

## 2. Hochschul-ERP — Ressourcenmanagement (what this dump contains)

HIS describes the Hochschul-ERP as "die Module des HIS-Finanz- und
Personalmanagements" with two *Bereiche*: **Finanzen** and **Personal**.

The dump's `db_version` tables record the concrete module identifiers:

| Schema | Module id (from `db_version`) | Version in dump | Domain |
|---|---|---|---|
| `sva4` | `HISSVA-GX` | 2026.06.0 | Personal- und Stellenverwaltung (HR & positions) |
| `mbs` | `HISMBS-UNIX`, `HISFSV-GX` | 6.0 / 2026.06.0 | Finanz- und Sachmittelverfahren (finance & resources) |
| `cob` | `HISCOB-GX` | 2026.06.0 | Kosten- und Leistungsrechnung (cost & activity accounting) |
| `kbs` | `HISKBS` | 2025.06.0 | Kassenbuchungssystem (cash-book / cashier) |

Other identifiers appearing across `db_version` rows: `HISFSV-GX`,
`HISSVA-GX`, `HISCOB-GX`, `HISMBS-UNIX`, plus `UPDATED` marker rows.

Observations (flagged as inference):
- The **`-GX`** suffix marks the newer generation of the ERP modules built on the
  HISinOne technology stack; `HISMBS-UNIX` (version `6.0`) is the legacy
  Unix-era module still present. The literal expansion of `GX` is not documented
  publicly — do not assume it.
- `FSV` appears in `mbs`, `cob` and `sva4` — it looks like a shared
  finance/accounting core referenced by several modules.
- Schema names are *not* 1:1 with product names (`sva4`, not `sva`; `mbs` hosts
  both `HISMBS` and `HISFSV`). Treat `db_version` as the authority, not the
  schema name.

## 3. Product-line summary

```
HIS eG
├── HISinOne (Campusmanagement)          ← NOT in this dump
│   ├── APP  Bewerbung & Zulassung
│   ├── STU  Studierendenmanagement
│   ├── EXA  Prüfungen & Veranstaltungen
│   ├── DOC  Doktorandenmanagement
│   ├── Forschungsmanagement
│   ├── Alumni / Portal / BI / SaaS
└── Hochschul-ERP (Ressourcenmanagement) ← THIS DUMP
    ├── sva4  HISSVA   Personal- und Stellenverwaltung
    ├── mbs   HISMBS / HISFSV   Finanz- und Sachmittelverfahren
    ├── cob   HISCOB   Kosten- und Leistungsrechnung
    └── kbs   HISKBS   Kassenbuchungssystem
```

## 4. Open point for the discussion

The task framing said "HISinOne". The dump is the **Hochschul-ERP**. Before any
further design work we must confirm which product the project targets, because
the two share no tables. See `open-questions.md` Q1.
