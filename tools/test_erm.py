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

# The legacy ER models are generated from schema/, whose contents are derived from a
# proprietary third-party database dump: scripts/analyze.sh reproduces them locally
# and they are not committed. Without them the generator still emits the target model
# and the legacy checks are skipped.
LEGACY_SCHEMAS = ("sva4", "mbs", "cob", "kbs")
LEGACY_AVAILABLE = (ROOT / "schema" / "tables_kbs.txt").is_file()

EXPECTED = {"target_canonical"}
if LEGACY_AVAILABLE:
    EXPECTED |= {f"legacy_{s}" for s in LEGACY_SCHEMAS}

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
        if LEGACY_AVAILABLE:
            for schema in LEGACY_SCHEMAS:
                check(f"legacy_{schema}" in produced, f"legacy {schema} ER model is produced")
        else:
            print("SKIP  legacy ER models (schema/ not present - run scripts/analyze.sh)")
        check(produced == EXPECTED, "exactly the expected ER models are produced")

        target = (out / "target_canonical.puml").read_text()
        entities = re.findall(r"^\s*entity\s+\"([^\"]+)\"", target, re.MULTILINE)
        # One entity per canonical table (migrations 0001-0029: 0017/0018 added the direct-debit
        # tables, 0022 the KLR allocation tables, 0024 the receivable, 0025 the cost types,
        # 0026 the Kostenträger keys and runs, 0027 the service rates and charges, 0028 the goods
        # receipts, 0029 the budget sources and centres).
        check(len(entities) == 48, f"target ER has 48 entities (got {len(entities)})")
        # The count alone cannot say *which* table a migration dropped, so the newest tables are
        # named: a migration whose table never reaches the model fails by name, not by arithmetic.
        for table in (
            "goods_receipt",
            "goods_receipt_line",
            "budget_source",
            "budget_center",
            "service_rate",
            "direct_debit_mandate",
        ):
            check(table in entities, f"the ER model carries the {table} table")
        fk_edges = re.findall(r"\|\|--o\{", target)
        check(len(fk_edges) >= 4, f"target ER has declared FK edges (got {len(fk_edges)})")
        check("[FK]" in target, "target ER marks foreign-key columns")

        for schema in LEGACY_SCHEMAS if LEGACY_AVAILABLE else ():
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
