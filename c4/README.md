# C4 model — Structurizr

`workspace.dsl` is the **authoritative** C4 model. It contains both the **as-is**
(legacy HIS-ERP) and the **target** (new platform), so the strangler path is visible
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

## Pitfalls (learned)

- The DSL is **line-oriented**: `systemContext s { include * }` on one line fails with
  "Too many tokens"; the `{` must open a block over multiple lines.
- Comments must be `//` (or `/* */`); `#` is **not** a comment and breaks the parse.
- The container reads the file as a non-root user: if `workspace.dsl` is mode `600`
  the parser reports only the bare filename as the error. Keep it `644`.
