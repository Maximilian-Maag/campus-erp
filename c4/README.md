# C4 model — Structurizr

`workspace.dsl` is the **authoritative** C4 model. It contains both the **as-is**
(legacy legacy university ERP) and the **target** (new platform), so the strangler path is visible
as a delta.

The Markdown files in `../docs/c4/` are narrative companions (rationale, evidence);
when they disagree with `workspace.dsl`, the DSL wins.

## View it

```bash
# interactive (Structurizr Lite) — http://localhost:8080
docker run --rm -p 8080:8080 \
  -v "$PWD/c4:/usr/local/structurizr" \
  structurizr/lite:latest
```

## Validate it (CI-friendly)

```bash
docker run --rm -v "$PWD/c4":/w -w /w \
  structurizr/structurizr:latest validate -w workspace.dsl
```

## Export diagrams

```bash
docker run --rm -v "$PWD/c4":/w -w /w -v "$PWD/c4/export":/out \
  structurizr/structurizr:latest export -w workspace.dsl -f mermaid -o /out
```

## Views

| Key | Level | Scope |
|---|---|---|
| `AsIs-Context` | 1 | legacy system context |
| `AsIs-Containers` | 2 | legacy containers |
| `AsIs-SvaComponents` | 3 | legacy framework components (replicated per schema) |
| `Target-Context` | 1 | target system context |
| `Target-Containers` | 2 | target containers |
| `Target-ApiComponents` | 3 | target API components (modular monolith) |

## Architecture report (C4 + ER models)

```bash
make c4-pdf            # full landscape report: C4 views + database ER models
make c4-pdf-no-erm     # C4 views only
make erm               # regenerate + render just the ER models
```

The report is `c4/export/campus-erp-c4-model.pdf` (landscape A4, one diagram per
page). It has two parts: **Part I** the rendered C4 views, **Part II** the ER
models of the databases. `tools/c4_pdf.sh` drives it; `c4/pdf/template.tex` is the
layout.

## ER models of the databases

`tools/gen_erm.py` generates PlantUML ER diagrams (`c4/erm/*.puml`) and
`tools/erm.sh` renders them to PNG. Two families:

| Model | Source | Relationships |
|---|---|---|
| `target_canonical` | the SQL migrations (`backend/internal/platform/db/migrations/`) | **declared** `FOREIGN KEY`s (read from the DDL) |
| `legacy_sva4` / `mbs` / `cob` / `kbs` | `schema/*.tsv` (measured PKs + columns) | **inferred** from `*_to_*` link tables and naming — the legacy schema has **zero** FKs |

The legacy `LEGACY_SPEC` (curated core entities per module) lives at the top of
`tools/gen_erm.py`; extend it there. Unknown table names are skipped with a
warning so a typo never breaks the build.

## Pitfalls (learned)

- The DSL is **line-oriented**: `systemContext s { include * }` on one line fails with
  "Too many tokens"; the `{` must open a block over multiple lines.
- Comments must be `//` (or `/* */`); `#` is **not** a comment and breaks the parse.
- The container reads the file as a non-root user: if `workspace.dsl` is mode `600`
  the parser reports only the bare filename as the error. Keep it `644`.
