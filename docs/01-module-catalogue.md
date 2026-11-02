# Module catalogue

The module identifiers below were read from the installed system itself.

The legacy platform ships several product lines. Two matter here:

1. **the reference implementation** — campus management (student lifecycle).
2. **the legacy university ERP** — "Ressourcenmanagement": finance, personnel, resources.

They are separate products that share technology and integrate, but they are
**not** the same database. The seed dump is a **the legacy university ERP** database.

---

## 1. the reference implementation — Campusmanagement (student lifecycle)

Organised by lifecycle stage; each stage is a module with a short code.

| Code | Module | Lifecycle stage / scope |
|---|---|---|
| `the reference implementation-APP` | Bewerbung und Zulassung (Application & Admission) | Information, Online-Bewerbung, Zulassung |
| `the reference implementation-STU` | Studierendenmanagement (Student management) | Immatrikulation, Rückmeldung, Beurlaubung, Exmatrikulation, Gebühren, Statistik, Krankenversicherung/SMV |
| `the reference implementation-EXA` | Prüfungen & Veranstaltungen (Exams & courses) | Curriculum Designer, Semester-/Veranstaltungsplanung, Prüfungen, Leistungsverbuchung, Praktika |
| `the reference implementation-DOC` | Doktorandenmanagement (Doctoral management) | Registrierung → Promotionsverfahren, Promovierendenstatistik |
| — | Alumni | Zentrales Portal für alle Zielgruppen |
| — | Forschungsmanagement (Research management) | KDSF-basierte Forschungsmetadaten |
| — | Portal | Central role-specific entry point |
| — | Business Intelligence | Kennzahlen, Management-Cockpit |
| — | hosted SaaS | Hosting/operations offering |

Cross-cutting platform features (apply to all modules): Dokumentenmanagement,
Leistungsstarke Suche, Identitätsmanagement & Verzeichnisdienst, Berichtscenter,
Barrierefreiheit (BITV 2.0 / WCAG), DSGVO-Unterstützung.

> The `APP/STU/EXA/DOC` codes are confirmed by the vendor's own product material
> (its catalogue describes the examination module by the same code).
> Alumni / Forschungsmanagement codes are not stated publicly and are left blank
> rather than guessed.

## 2. the legacy university ERP — Ressourcenmanagement (what this dump contains)

The product material describes the legacy university ERP as "die Module des Finanz- und
Personalmanagements" with two *Bereiche*: **Finanzen** and **Personal**.

The dump's `db_version` tables record the concrete module identifiers:

| Schema | Module id (from `db_version`) | Version in dump | Domain |
|---|---|---|---|
| `sva4` | `the HR module-GX` | 2026.06.0 | Personal- und Stellenverwaltung (HR & positions) |
| `mbs` | `the finance module-UNIX`, `the finance module-GX` | 6.0 / 2026.06.0 | Finanz- und Sachmittelverfahren (finance & resources) |
| `cob` | `the cost-accounting module-GX` | 2026.06.0 | Kosten- und Leistungsrechnung (cost & activity accounting) |
| `kbs` | `the cash-book module` | 2025.06.0 | Kassenbuchungssystem (cash-book / cashier) |

Other identifiers appearing across `db_version` rows: `the finance module-GX`,
`the HR module-GX`, `the cost-accounting module-GX`, `the finance module-UNIX`, plus `UPDATED` marker rows.

Observations (flagged as inference):
- The **`-GX`** suffix marks the newer generation of the ERP modules built on the
  the reference implementation technology stack; `the finance module-UNIX` (version `6.0`) is the legacy
  Unix-era module still present. The literal expansion of `GX` is not documented
  publicly — do not assume it.
- `FSV` appears in `mbs`, `cob` and `sva4` — it looks like a shared
  finance/accounting core referenced by several modules.
- Schema names are *not* 1:1 with product names (`sva4`, not `sva`; `mbs` hosts
  both `the finance module` and `the finance module`). Treat `db_version` as the authority, not the
  schema name.

## 3. Product-line summary

```
the vendor
├── the reference implementation (Campusmanagement)          ← NOT in this dump
│   ├── APP  Bewerbung & Zulassung
│   ├── STU  Studierendenmanagement
│   ├── EXA  Prüfungen & Veranstaltungen
│   ├── DOC  Doktorandenmanagement
│   ├── Forschungsmanagement
│   ├── Alumni / Portal / BI / SaaS
└── the legacy university ERP (Ressourcenmanagement) ← THIS DUMP
    ├── sva4  the HR module   Personal- und Stellenverwaltung
    ├── mbs   the finance module / the finance module   Finanz- und Sachmittelverfahren
    ├── cob   the cost-accounting module   Kosten- und Leistungsrechnung
    └── kbs   the cash-book module   Kassenbuchungssystem
```

## 4. Open point for the discussion

The task framing said "the reference implementation". The dump is the **the legacy university ERP**. Before any
further design work we must confirm which product the project targets, because
the two share no tables. See `open-questions.md` Q1.
