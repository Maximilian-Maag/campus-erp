#!/usr/bin/env python3
"""Generate Entity-Relationship (ERM) PlantUML diagrams for campus-erp.

Two families of databases exist in this project and both are modelled here:

  * target  — the new canonical schema, defined by the SQL migrations in
    backend/internal/platform/db/migrations/. It has real PRIMARY KEY /
    FOREIGN KEY constraints, so the ER model is read *directly* from the DDL
    (no guessing). One database per tenant (ADR-0002).

  * legacy  — the reverse-engineered HIS-ERP schemas (sva4, mbs, cob, kbs).
    These have **zero declared foreign keys** (see docs/02-database-overview.md),
    so the model is reconstructed from measured metadata (PKs, column lists in
    schema/*.tsv) plus the naming conventions documented in
    docs/03-schema-analysis.md. Every edge in the legacy diagrams is *inferred*;
    the diagrams say so in their legend.

Output: c4/erm/*.puml, consumed by tools/erm.sh (PlantUML docker) -> c4/export/*.png.

Usage:
    tools/gen_erm.py [--out DIR] [--root DIR]
"""
from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

# ──────────────────────────────────────────────────────────────────────────────
# shared helpers
# ──────────────────────────────────────────────────────────────────────────────


def esc(name: str) -> str:
    """Escape a PlantUML string literal (double-quote safe)."""
    return name.replace('"', "'")


def ident(name: str) -> str:
    """A PlantUML-safe alias for a potentially dotted/schemad name."""
    return re.sub(r"[^0-9A-Za-z_]", "_", name)


# ──────────────────────────────────────────────────────────────────────────────
# target schema — parsed from migrations
# ──────────────────────────────────────────────────────────────────────────────


@dataclass
class Column:
    name: str
    type: str
    not_null: bool = False
    pk: bool = False
    fk: str | None = None  # "table(col)" if a REFERENCES clause was seen


@dataclass
class Table:
    name: str
    columns: list[Column] = field(default_factory=list)
    pk: list[str] = field(default_factory=list)
    comment: list[str] = field(default_factory=list)

    def col(self, name: str) -> Column | None:
        for c in self.columns:
            if c.name == name:
                return c
        return None


_TABLE_RE = re.compile(
    r"CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"
    r'(?P<name>[A-Za-z_][\w.]*)\s*\(',
    re.IGNORECASE,
)
_REFERENCES_RE = re.compile(
    r"REFERENCES\s+(?P<tbl>[A-Za-z_][\w.]*)\s*\(\s*(?P<col>[A-Za-z_]\w*)\s*\)",
    re.IGNORECASE,
)
_PK_TABLE_RE = re.compile(
    r"(?:CONSTRAINT\s+\w+\s+)?PRIMARY\s+KEY\s*\((?P<cols>[^)]*)\)",
    re.IGNORECASE,
)


def _matching_paren(text: str, open_idx: int) -> int:
    """Index of the '(' matching the one at open_idx, honouring '...' quotes."""
    depth = 0
    i = open_idx
    in_str = False
    while i < len(text):
        ch = text[i]
        if in_str:
            if ch == "'":
                if i + 1 < len(text) and text[i + 1] == "'":
                    i += 1
                else:
                    in_str = False
        else:
            if ch == "'":
                in_str = True
            elif ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
                if depth == 0:
                    return i
        i += 1
    raise ValueError("unbalanced parentheses")


def _split_top_level(body: str) -> list[str]:
    """Split a CREATE TABLE body on top-level commas."""
    parts, depth, cur, in_str = [], 0, [], False
    for i, ch in enumerate(body):
        if in_str:
            cur.append(ch)
            if ch == "'":
                if i + 1 < len(body) and body[i + 1] == "'":
                    pass
                else:
                    in_str = False
            continue
        if ch == "'":
            in_str = True
            cur.append(ch)
        elif ch == "(":
            depth += 1
            cur.append(ch)
        elif ch == ")":
            depth -= 1
            cur.append(ch)
        elif ch == "," and depth == 0:
            parts.append("".join(cur))
            cur = []
        else:
            cur.append(ch)
    if cur:
        parts.append("".join(cur))
    return [p.strip() for p in parts if p.strip()]


_KEYWORDS = {
    "PRIMARY", "FOREIGN", "UNIQUE", "CHECK", "CONSTRAINT", "KEY", "REFERENCES",
    "GENERATED", "NOT", "NULL", "DEFAULT", "IDENTITY", "ALWAYS", "AS", "COLLATE",
}


def parse_migrations(path: Path) -> list[Table]:
    """Parse every CREATE TABLE in the migration directory."""
    tables: dict[str, Table] = {}
    sql_files = sorted(path.glob("*.sql"))
    for sql_file in sql_files:
        text = sql_file.read_text(encoding="utf-8")
        # leading comment block of the file, used as the per-slice description
        file_comment = []
        for line in text.splitlines():
            s = line.strip()
            if s.startswith("--"):
                file_comment.append(s[2:].strip())
            elif s:
                break

        for m in _TABLE_RE.finditer(text):
            name = m.group("name").split(".")[-1]
            close = _matching_paren(text, m.end() - 1)
            body = text[m.end():close]
            tbl = Table(name=name, comment=list(file_comment))

            for item in _split_top_level(body):
                first = item.split(None, 1)[0].upper().rstrip(",")
                if first in _KEYWORDS:
                    pk = _PK_TABLE_RE.search(item)
                    if pk:
                        tbl.pk += [c.strip() for c in pk.group("cols").split(",")]
                    continue
                toks = item.split(None, 1)
                if len(toks) < 2:
                    continue
                cname, rest = toks[0], toks[1]
                # column type = up to the first constraint keyword
                words, ctype = rest.split(), []
                for w in words:
                    if w.upper() in _KEYWORDS:
                        break
                    ctype.append(w)
                col = Column(
                    name=cname,
                    type=" ".join(ctype) or "?",
                    not_null="NOT NULL" in item.upper(),
                    pk="PRIMARY KEY" in item.upper(),
                )
                ref = _REFERENCES_RE.search(item)
                if ref:
                    col.fk = f"{ref.group('tbl').split('.')[-1]}({ref.group('col')})"
                tbl.columns.append(col)

            for c in tbl.columns:
                if c.name in tbl.pk:
                    c.pk = True
            tables[name] = tbl
    return list(tables.values())


def target_puml(tables: list[Table]) -> str:
    aliases = {t.name: ident("t_" + t.name) for t in tables}
    lines = [
        "@startuml",
        "!theme plain",
        "hide circle",
        "skinparam linetype ortho",
        "skinparam entity {",
        "  BackgroundColor #EAF2F8",
        "  BorderColor #0B4A6F",
        "  HeaderBackgroundColor #0B4A6F",
        "  HeaderFontColor #FFFFFF",
        "}",
        "skinparam defaultFontSize 11",
        'title campus-erp — target canonical schema (one database per tenant, ADR-0002)',
        "",
    ]
    for t in tables:
        lines.append(f'entity "{esc(t.name)}" as {aliases[t.name]} {{')
        pk_cols = [c for c in t.columns if c.pk]
        other = [c for c in t.columns if not c.pk]
        for c in pk_cols:
            lines.append(f"  * {c.name} : {c.type} [PK]")
        if pk_cols and other:
            lines.append("  --")
        for c in other:
            mark = " [FK]" if c.fk else ""
            star = "*" if c.not_null else " "
            lines.append(f"  {star} {c.name} : {c.type}{mark}")
        lines.append("}")
        lines.append("")

    edges = []
    for t in tables:
        for c in t.columns:
            if not c.fk:
                continue
            ref_table = c.fk.split("(")[0]
            if ref_table not in aliases:
                continue
            edges.append(
                f'{aliases[ref_table]} ||--o{{ {aliases[t.name]} : "{esc(c.name)}"'
            )
    lines += ["' ── foreign keys (declared in the migrations) ──", *edges]
    lines += ["", "legend right", "solid crow's foot = declared FOREIGN KEY", "endlegend", "@enduml", ""]
    return "\n".join(lines)


# ──────────────────────────────────────────────────────────────────────────────
# legacy schemas — curated core entities + inferred edges
# ──────────────────────────────────────────────────────────────────────────────

# Entity groups per module. Names must exist in schema/tables_<schema>.txt;
# unknown names are skipped with a warning so the file never breaks the build.
LEGACY_SPEC: dict[str, dict] = {
    "sva4": {
        "module": "HISSVA-GX",
        "title": "sva4 — Personal- und Stellenverwaltung",
        "colour": "#E8F6EF",
        "groups": {
            "Person & Bezüge (PB*)": ["pgd", "pgp", "pbv", "pbw", "pbz", "pbe", "pbh", "pbl"],
            "Sozialversicherung": ["sas", "sas_stand", "sbu", "sko", "shv"],
            "Struktur, Kosten & Einrichtung": ["bgr", "sem", "sgd", "__inst"],
            "Bezahlung / geldwerte Vorteile": ["gvp", "gvp_vert"],
            "Externe Systeme": ["meta_person", "meta_rolle", "qis_nutzer"],
        },
        # documented in docs/03-schema-analysis.md §1 (inferred, no FKs)
        "extra_edges": [("gvp", "pbv"), ("gvp", "sgd"), ("sem", "pgd"), ("pgd", "bgr")],
    },
    "mbs": {
        "module": "HISMBS-UNIX / HISFSV-GX",
        "title": "mbs — Finanz- und Sachmittelverfahren",
        "colour": "#FDF3E3",
        "groups": {
            "FiBu / Haushalt": ["fibu_konten", "fibu_journal", "fibu_debitoren", "fibu_kreditoren", "rechnungseingangsbuch"],
            "Beschaffung / Vergabe": ["vergabe", "anfr", "anfp", "ange", "angp", "auft", "lieferant"],
            "Liegenschaften (glm_*)": ["glm_ek", "glm_se", "glm_sw"],
            "Inventar (iv*)": ["ivab", "ivst"],
            "Personalkosten": ["pers", "ein_personal"],
            "Banking / SEPA": ["bic_sepa_credit_transfer", "direct_debit_mandate"],
        },
        "extra_edges": [],
    },
    "cob": {
        "module": "HISCOB-GX",
        "title": "cob — Kosten- und Leistungsrechnung",
        "colour": "#F4ECF7",
        "groups": {
            "Haushaltsstruktur (BUSA)": ["busa", "arch_busa", "alt_hier"],
            "Bezugsgrößen": ["bzgr"],
            "Umlage / Verrechnung": ["kpum", "umks", "umvm"],
            "Projekte & Studierende": ["proj", "stug"],
            "Ansatz / Drittmittel": ["ansc", "drittmittelherk"],
        },
        "extra_edges": [],
    },
    "kbs": {
        "module": "HISKBS",
        "title": "kbs — Kassenbuchungssystem",
        "colour": "#FDEDEC",
        "groups": {
            "Kassenbuch": ["kass", "schalterbuch", "sb", "bu", "vorg"],
            "Anordnungen": ["ao"],
            "Zahlungsverkehr": ["zawe", "zabe", "bank_sctsdd"],
            "Mahnwesen": ["mahn", "mahnprot"],
            "Stiftungen / Finanzamt": ["stift", "finanzamt", "mwst_art"],
        },
        # documented in docs/03 §4 (inferred — the legacy schema has no FKs)
        "extra_edges": [("kass", "bu"), ("mahn", "ao"), ("zabe", "zawe")],
    },
}


@dataclass
class LegacyData:
    tables: set[str]
    pk: dict[str, list[str]]
    columns: dict[str, list[tuple[str, str]]]
    link_edges: list[tuple[str, str, str]]  # (a, b, link_table)


def load_legacy(schema_dir: Path) -> dict[str, LegacyData]:
    out: dict[str, LegacyData] = {}
    for schema in LEGACY_SPEC:
        tfile = schema_dir / f"tables_{schema}.txt"
        if not tfile.exists():
            continue
        tables = {ln.strip() for ln in tfile.read_text().splitlines() if ln.strip()}

        pk: dict[str, list[str]] = {}
        for ln in (schema_dir / "pks.tsv").read_text().splitlines():
            parts = ln.split("\t")
            if len(parts) == 3 and parts[0] == schema:
                pk[parts[1]] = [c.strip() for c in parts[2].split(",") if c.strip()]

        columns: dict[str, list[tuple[str, str]]] = {}
        for ln in (schema_dir / "columns.tsv").read_text().splitlines():
            parts = ln.split("\t")
            if len(parts) >= 5 and parts[0] == schema:
                columns.setdefault(parts[1], []).append((parts[3], parts[4]))

        # infer many-to-many seams from the *_to_* link tables (naming convention)
        link_edges = []
        for t in sorted(tables):
            base = t[2:] if t.startswith("__") else t
            m = re.match(r"^(?P<a>.+?)_to_(?P<b>.+)$", base)
            if not m:
                continue
            a, b = m.group("a"), m.group("b")
            if a in tables and b in tables:
                link_edges.append((a, b, t))
        out[schema] = LegacyData(tables, pk, columns, link_edges)
    return out


AUDIT_COLS = {"db_einfuegemarke", "db_aendmarke", "db_loeschmarke"}


def legacy_puml(schema: str, data: LegacyData) -> str:
    spec = LEGACY_SPEC[schema]
    lines = [
        "@startuml",
        "!theme plain",
        "hide circle",
        "skinparam linetype ortho",
        "skinparam entity {",
        f"  BackgroundColor {spec['colour']}",
        "  BorderColor #5D6D7E",
        "  HeaderBackgroundColor #34495E",
        "  HeaderFontColor #FFFFFF",
        "}",
        "skinparam defaultFontSize 11",
        f'title campus-erp — legacy {schema} : {spec["title"]}\\nmodule {spec["module"]} — reverse-engineered, NO foreign keys exist',
        "",
    ]

    # gather curated entities, warn on typos
    wanted: list[str] = []
    for grp in spec["groups"].values():
        wanted += grp
    usable = [t for t in wanted if t in data.tables]
    missing = [t for t in wanted if t not in data.tables]
    for m in missing:
        print(f"  warn: {schema}: curated table '{m}' not in schema — skipped", file=sys.stderr)

    for title, group in spec["groups"].items():
        members = [t for t in group if t in data.tables]
        if not members:
            continue
        pkg = ident("g_" + title)
        lines.append(f'package "{esc(title)}" as {pkg} {{')
        for t in members:
            alias = ident("t_" + t)
            lines.append(f'  entity "{esc(t)}" as {alias} {{')
            pk = data.pk.get(t, [])
            cols = data.columns.get(t, [])
            shown = [(n, ty) for n, ty in cols if n not in AUDIT_COLS]
            pk_rows = [(n, ty) for n, ty in shown if n in pk]
            other = [(n, ty) for n, ty in shown if n not in pk][:4]
            for n, ty in pk_rows:
                lines.append(f"    * {n} : {ty} [PK]")
            if pk_rows and other:
                lines.append("    --")
            for n, ty in other:
                lines.append(f"    {n} : {ty}")
            extra = len(shown) - len(pk_rows) - len(other)
            if extra > 0:
                lines.append(f"    … +{extra} further column(s)")
            lines.append("  }")
        lines.append("}")
        lines.append("")

    # edges: inferred *_to_* seams, only between curated entities
    edges = []
    for a, b, link in data.link_edges:
        if a in usable and b in usable:
            edges.append(
                f'{ident("t_" + a)} }}o--o{{ {ident("t_" + b)} : "{esc(link)}"'
            )
    for a, b in spec["extra_edges"]:
        if a in usable and b in usable:
            edges.append(f'{ident("t_" + a)} }}o--{{ {ident("t_" + b)} : "inferred"')
    if edges:
        lines += ["' ── inferred relationships (naming convention + *_to_* links) ──", *edges]

    lines += [
        "",
        "legend right",
        "no FOREIGN KEY constraints exist in the legacy schema",
        "every edge is INFERRED from *_to_* link tables / naming",
        "endlegend",
        "@enduml",
        "",
    ]
    return "\n".join(lines)


# ──────────────────────────────────────────────────────────────────────────────


def _rel(p: Path, root: Path) -> str:
    """Path relative to root when possible, else the absolute path."""
    try:
        return str(p.relative_to(root))
    except ValueError:
        return str(p)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    here = Path(__file__).resolve().parent.parent
    ap.add_argument("--root", type=Path, default=here)
    ap.add_argument("--out", type=Path, default=None, help="output dir (default c4/erm)")
    args = ap.parse_args()

    root: Path = args.root
    out_dir = args.out or (root / "c4" / "erm")
    out_dir.mkdir(parents=True, exist_ok=True)

    written: list[Path] = []

    # 1. target schema from migrations
    mig = root / "backend" / "internal" / "platform" / "db" / "migrations"
    if mig.is_dir():
        tables = parse_migrations(mig)
        if tables:
            p = out_dir / "target_canonical.puml"
            p.write_text(target_puml(tables), encoding="utf-8")
            written.append(p)
            print(f"target : {len(tables)} table(s) -> {_rel(p, root)}")
    else:
        print(f"warn: no migrations dir at {mig}", file=sys.stderr)

    # 2. legacy schemas
    schema_dir = root / "schema"
    legacy = load_legacy(schema_dir)
    for schema in LEGACY_SPEC:
        if schema not in legacy:
            print(f"warn: no schema data for '{schema}'", file=sys.stderr)
            continue
        p = out_dir / f"legacy_{schema}.puml"
        p.write_text(legacy_puml(schema, legacy[schema]), encoding="utf-8")
        written.append(p)
        print(f"legacy : {schema} -> {_rel(p, root)}")

    print(f"ok: {len(written)} ER model(s) in {out_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
