#!/usr/bin/env python3
"""Self-test for tools/gen_erm.py (the ER-model generator).

Guards the two things most likely to rot:
  * the target ER model stays in sync with the migrations (entity + FK counts);
  * the legacy diagrams never gain a *fabricated* relationship (the legacy schema
    has zero foreign keys, so every edge must be labelled inferred).

Run directly (`tools/test_erm.py`) or via `tools/run_tests.sh` / `make test-tools`.
"""
from __future__ import annotations

import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GEN = ROOT / "tools" / "gen_erm.py"

EXPECTED = {
    "target_canonical",
    "legacy_sva4",
    "legacy_mbs",
    "legacy_cob",
    "legacy_kbs",
}

failures: list[str] = []


def check(cond: bool, msg: str) -> None:
    if cond:
        print(f"PASS  {msg}")
    else:
        print(f"FAIL  {msg}")
        failures.append(msg)


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        out = Path(tmp)
        rc = subprocess.run(
            [sys.executable, str(GEN), "--root", str(ROOT), "--out", str(out)],
            capture_output=True, text=True,
        )
        check(rc.returncode == 0, "gen_erm.py exits 0")
        if rc.returncode != 0:
            print(rc.stderr)
            return 1

        produced = {p.stem for p in out.glob("*.puml")}
        check("target_canonical" in produced, "target ER model is produced")
        for schema in ("sva4", "mbs", "cob", "kbs"):
            check(f"legacy_{schema}" in produced, f"legacy {schema} ER model is produced")
        check(produced == EXPECTED, "exactly the expected ER models are produced")

        target = (out / "target_canonical.puml").read_text()
        entities = re.findall(r"^\s*entity\s+\"", target, re.MULTILINE)
        # +0011(4) = 25 tables
        check(len(entities) == 25, f"target ER has 21 entities (got {len(entities)})")
        fk_edges = re.findall(r"\|\|--o\{", target)
        check(len(fk_edges) >= 4, f"target ER has declared FK edges (got {len(fk_edges)})")
        check("[FK]" in target, "target ER marks foreign-key columns")

        for schema in ("sva4", "mbs", "cob", "kbs"):
            txt = (out / f"legacy_{schema}.puml").read_text()
            check(
                "INFERRED" in txt.upper(),
                f"legacy {schema} declares edges are inferred (no FKs exist)",
            )
            # guard against accidental solid "declared" styling in legacy diagrams
            check("declared FOREIGN KEY" not in txt, f"legacy {schema} claims no declared FKs")

    if failures:
        print(f"\n{len(failures)} check(s) failed")
        return 1
    print("\nall ER generator checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
