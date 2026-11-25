#!/usr/bin/env python3
"""gen_language — render the ubiquitous language as readable markdown.

docs/ubiquitous-language.json is the source of truth. This script renders
docs/ubiquitous-language.md from it, so the readable page and the enforced
rules cannot drift apart. Run `make language-gen` after editing the JSON;
`make language` fails when the markdown is stale.

Usage:
    python3 tools/gen_language.py            # write the markdown
    python3 tools/gen_language.py --check    # exit 1 when it is stale
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "docs" / "ubiquitous-language.json"
TARGET = ROOT / "docs" / "ubiquitous-language.md"


def render(data: dict) -> str:
    ortho = data["orthography"]
    lines = [
        "# Ubiquitous language",
        "",
        "Generated from `docs/ubiquitous-language.json` by `tools/gen_language.py`.",
        "Do not edit this page; edit the JSON and run `make language-gen`.",
        "",
        "One concept, one name. The same word is used in the requirements, the C4",
        "model, the API contract, the database, the code and the German UI copy. A",
        "concept that needs two names is two concepts.",
        "",
        "## Orthography",
        "",
        "The same concept is spelled for its layer. Neither layer may mix the two.",
        "",
        "| layer | canonical | never | why |",
        "| --- | --- | --- | --- |",
    ]
    for layer in ("prose", "code"):
        rule = ortho[layer]
        avoid = rule["avoid"]
        if isinstance(avoid, list):
            avoid = ", ".join(f"`{a}`" for a in avoid)
        else:
            avoid = f"`{avoid}`"
        lines.append(f"| {layer} | `{rule['canonical']}` | {avoid} | {rule['why']} |")

    lines += [
        "",
        "## Terms",
        "",
        "| concept | German | definition | never |",
        "| --- | --- | --- | --- |",
    ]
    for term in data["terms"]:
        avoid = ", ".join(f"`{a}`" for a in term.get("avoid", [])) or "—"
        lines.append(
            f"| {term['concept']} | {term['german']} | {term['definition']} | {avoid} |"
        )

    lines += [
        "",
        "## Where it is enforced",
        "",
        "`tools/check_language.py` reads the JSON and scans every tracked text file.",
        "It runs as `make language` and in CI. Prose is scanned with code spans and",
        "fenced blocks removed, so an endpoint such as `/api/cost-centres` is not",
        "mistaken for prose. The historical migrations and the legacy analysis keep",
        "their original spelling: they describe the system this product replaces.",
        "",
    ]
    return "\n".join(lines)


def label(path: Path) -> str:
    """Repo-relative when possible, absolute when the target lives outside ROOT."""
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)


def main() -> int:
    check = "--check" in sys.argv[1:]
    data = json.loads(SOURCE.read_text(encoding="utf-8"))
    rendered = render(data)
    if check:
        current = TARGET.read_text(encoding="utf-8") if TARGET.exists() else ""
        if current != rendered:
            print(f"FAIL {label(TARGET)} is stale; run make language-gen")
            return 1
        print(f"PASS {label(TARGET)} is current")
        return 0
    TARGET.write_text(rendered, encoding="utf-8")
    print(f"wrote {label(TARGET)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
