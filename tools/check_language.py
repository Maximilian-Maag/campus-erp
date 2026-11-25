#!/usr/bin/env python3
"""check_language — enforce the ubiquitous language mechanically.

docs/ubiquitous-language.json is the source of truth. This script scans every
tracked text file and fails when a layer uses a word the language forbids:
the wrong orthography, or a synonym that names the same concept twice.

Usage:
    python3 tools/check_language.py            # every tracked file
    python3 tools/check_language.py --changed  # only files changed locally
    python3 tools/check_language.py --json     # machine-readable result

Exit status: 1 when any rule fails.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "docs" / "ubiquitous-language.json"

# Files that describe the system this product replaces, or that are generated.
# They keep the spelling they were written with; the language governs the target.
HISTORY = (
    "backend/internal/platform/db/migrations/",
    "docs/legacy/",
    "CHANGELOG.md",
)
GENERATED = (
    "frontend/src/app/core/api/generated/",
    "contracts/openapi.yaml",
    "docs/ubiquitous-language.md",
    "docs/ubiquitous-language.json",
    "backend/internal/httpapi/domaincodes.go",
)

# CSS alignment values are not the domain concept; a stylesheet may centre a box.
CSS_CENTRE = re.compile(r"(align-items|justify-content|place-items|text-align):\s*center")
PROSE_SUFFIXES = (".md",)
CODE_SUFFIXES = (".go", ".ts", ".html", ".sql", ".yaml", ".yml")

CODE_SPAN = re.compile(r"`[^`]*`")
FENCE = re.compile(r"```.*?```", re.S)


def strip_markdown(text: str) -> str:
    """Blank fenced blocks and inline code so identifiers are not read as prose."""
    text = FENCE.sub("", text)
    return CODE_SPAN.sub("", text)


def tracked_files(changed: bool) -> list[str]:
    cmd = ["git", "ls-files"]
    if changed:
        cmd = ["git", "diff", "--name-only", "HEAD"]
    out = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, check=True)
    return [line for line in out.stdout.splitlines() if line.strip()]


def classify(path: str) -> str | None:
    if any(path.startswith(prefix) for prefix in HISTORY):
        return None
    if any(path.startswith(prefix) for prefix in GENERATED):
        return None
    if path.endswith(PROSE_SUFFIXES):
        return "prose"
    if path.endswith(CODE_SUFFIXES):
        return "code"
    return None


def word_pattern(phrase: str) -> re.Pattern[str]:
    """Match a phrase as whole words, tolerant of case."""
    parts = [re.escape(part) for part in phrase.split()]
    return re.compile(r"(?<!\w)" + r"\s+".join(parts) + r"(?!\w)", re.IGNORECASE)


def scan(path: str, text: str, layer: str, language: dict) -> list[str]:
    findings: list[str] = []
    ortho = language["orthography"]
    rule = ortho["prose"] if layer == "prose" else ortho["code"]
    avoid = rule["avoid"]
    if isinstance(avoid, str):
        avoid = [avoid]
    haystack = strip_markdown(text) if layer == "prose" else text
    if layer == "code":
        # Blank the CSS declaration rather than dropping the line, so line numbers stay true.
        haystack = "\n".join(CSS_CENTRE.sub("", line) for line in haystack.splitlines())
    for phrase in avoid:
        for match in word_pattern(phrase).finditer(haystack):
            line = haystack.count("\n", 0, match.start()) + 1
            findings.append(
                f"{path}:{line}: '{phrase}' is not the language; use '{rule['canonical']}'"
            )
    for term in language["terms"]:
        for phrase in term.get("avoid", []):
            for match in word_pattern(phrase).finditer(haystack):
                line = haystack.count("\n", 0, match.start()) + 1
                findings.append(
                    f"{path}:{line}: '{phrase}' names the same concept as "
                    f"'{term['concept']}'; use the concept's name"
                )
    return findings


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--changed", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()

    language = json.loads(SOURCE.read_text(encoding="utf-8"))
    findings: list[str] = []
    scanned = 0
    for path in tracked_files(args.changed):
        layer = classify(path)
        if layer is None:
            continue
        file = ROOT / path
        try:
            text = file.read_text(encoding="utf-8")
        except (UnicodeDecodeError, FileNotFoundError):
            continue
        scanned += 1
        findings.extend(scan(path, text, layer, language))

    if args.json:
        print(json.dumps({"scanned": scanned, "findings": findings}, indent=2))
    else:
        print(f"scanned {scanned} files")
        for finding in findings:
            print(f"FAIL {finding}")
        if not findings:
            print("PASS ubiquitous language holds in every layer")
    return 1 if findings else 0


if __name__ == "__main__":
    raise SystemExit(main())
