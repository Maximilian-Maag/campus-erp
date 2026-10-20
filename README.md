# HISinOne / HIS-ERP — Database Reverse-Engineering

Private working repository. Starting context for a project discussion around the
HIS eG higher-education management software suite.

## What this repository is

A reverse-engineered description of the database delivered in the seed dump
`hisrm_muster_30_09_26.backup`, plus a catalogue of the HIS product's modules,
a C4 model, and a requirements baseline.

It is a **documentation + analysis** repository. It contains no application code.
All artefacts under `schema/` are machine-generated from the dump and can be
reproduced with `scripts/`.

## The dump

| Property | Value |
|---|---|
| File | `hisrm_muster_30_09_26.backup` |
| Size | 15,648,251 bytes (~15 MB) |
| Format | PostgreSQL custom archive, format version `1.16` |
| Produced by | `pg_dump` 17.9 (Ubuntu 17.9-1.pgdg24.04+1) |
| Database | `hisrm_muster` |
| Encoding / locale | UTF8 / `de_DE.UTF-8` (restored here with `C` collation) |
| Dump date hint | file dated 2025-09-30 (`30_09_26` in name) |

> **Important distinction.** The user-facing name "HISinOne" is the HIS eG
> *campus management* product line (modules APP / STU / EXA / DOC / …).
> This dump does **not** contain the campus-management schemas. It contains the
> **Hochschul-ERP** ("Ressourcenmanagement") modules — Personal, Finanzen,
> Sachmittel, Kasse, Kosten-/Leistungsrechnung. See
> [`docs/01-module-catalogue.md`](docs/01-module-catalogue.md) for the full
> mapping. This is the single most important finding to resolve with the project
> sponsor.

## Contents at a glance

| Schema | Module (from `db_version`) | Meaning | Tables |
|---|---|---|---|
| `sva4` | `HISSVA-GX` | Personal- und Stellenverwaltung | 636 |
| `mbs` | `HISMBS-UNIX` / `HISFSV-GX` | Finanz- und Sachmittelverfahren | 556 |
| `cob` | `HISCOB-GX` | Kosten- und Leistungsrechnung | 244 |
| `kbs` | `HISKBS` | Kassenbuchungssystem | 94 |
| `hisrm` | (core/framework) | shared core | 3 |
| `public`, `histest`, `test` | — | bootstrap / test | 1 each |

Totals: **1,536 base tables**, 374 views, 1,117 sequences, 3,601 functions,
1,439 triggers, 4,352 indexes, 38,453 columns.

## Repository layout

```
README.md                     this file
AGENTS.md                     repo rules (source of the policy checks)
CHANGELOG.md                  change log
policy.json                   policy-as-code configuration
docs/
  01-module-catalogue.md      HIS product & module catalogue
  02-database-overview.md     schemas, sizes, conventions, framework layer
  03-schema-analysis.md       reverse-engineered data model
  04-data-dictionary.md       how to read the generated dictionary
  glossary.md                 German→English domain terms
  requirements.md             requirements baseline (living)
  open-questions.md           questions to resolve in the discussion
  c4/                          C4 model: context, container, component, code
schema/                       GENERATED artefacts (DDL, inventories)
scripts/                      restore + analysis scripts (reproducibility)
tools/                        vendored policy checker
```

## Reproducing

```bash
# 1. start a sandbox Postgres and restore the dump
scripts/restore.sh /path/to/hisrm_muster_30_09_26.backup

# 2. regenerate the schema artefacts
scripts/analyze.sh
```

## Status

Context-gathering phase. The module catalogue and schema analysis are complete
enough to start the project discussion; `docs/requirements.md` and
`docs/open-questions.md` are deliberately provisional and will be filled in
during that discussion.
