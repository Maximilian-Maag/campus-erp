#!/usr/bin/env python3
"""Self-test for tools/gen_domaincodes.py (the domain-error → machine-code generator).

The generator is a CI guard (`make contracts-check` runs it with --check), so the guard itself needs
one: if its parser silently stopped matching the statusFor… functions, the table would stay as it was
and a new refusal would reach a client without a code. What this checks:

  * the code shape — package + snake_case, and `VATType` does not become `v_a_t_type`;
  * the table is generated from the status functions and is not empty;
  * no code is claimed twice (the generator refuses that, and this proves it);
  * the file on disk is what the generator would write — drift is a failure here too, so
    `make test-tools` catches it even when the contract check is not run.

Run directly (`tools/test_domaincodes.py`) or via `make test-tools`.
"""
from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "tools"))

import gen_domaincodes as gen  # noqa: E402

failures: list[str] = []


def check(name: str, got, want) -> None:
    if got != want:
        failures.append(f"{name}: got {got!r}, want {want!r}")


def snake_cases() -> None:
    cases = {
        "ErrMandateNotActive": "mandate_not_active",
        "ErrVATTypeExists": "vat_type_exists",
        "ErrIBANInvalid": "iban_invalid",
        "ErrOverReceipt": "over_receipt",
        "ErrX": "x",
        "ErrAllocationCycleDetected": "allocation_cycle_detected",
    }
    for name, want in cases.items():
        check(f"snake({name})", gen.snake(name), want)


def table_shape() -> None:
    entries = gen.collect()
    if not entries:
        failures.append("collect() found no domain error — the parser stopped matching")
        return
    # A known pair that a status function maps; if this disappears the parse is wrong.
    if ("kasse", "ErrActorRequired") not in entries:
        failures.append("collect() does not carry kasse.ErrActorRequired")
    if entries != sorted(entries):
        failures.append("collect() is not sorted, so the file would churn between runs")

    rendered = gen.render(entries)
    codes = [line.split('code: "')[1].split('"')[0] for line in rendered.splitlines() if "{err:" in line]
    if len(codes) != len(set(codes)):
        duplicated = sorted({c for c in codes if codes.count(c) > 1})
        failures.append(f"duplicate codes in the generated table: {duplicated}")
    if any("." not in c for c in codes):
        failures.append(f"a code is not <package>.<name>: {[c for c in codes if '.' not in c]}")
    if "package httpapi" not in rendered or "import (" not in rendered:
        failures.append("the generated file is not a Go file with imports")
    if "DO NOT EDIT" not in rendered:
        failures.append("the generated file does not say it is generated")


def duplicate_refusal() -> None:
    """Two errors that would collide must fail the generator rather than pick one."""
    # kasse and klr are different packages, so a collision needs the same package twice under two
    # spellings — which cannot happen in practice; what can happen is the same *code* from one entry
    # rendered twice, so that is what is exercised here.
    try:
        gen.render([("kasse", "ErrActorRequired"), ("kasse", "ErrActorRequired")])
    except SystemExit:
        return
    failures.append("render() accepted the same error twice instead of refusing a duplicate code")


def on_disk_is_current() -> None:
    wanted = gen.render(gen.collect())
    current = gen.OUT.read_text(encoding="utf-8") if gen.OUT.exists() else ""
    if current != wanted:
        failures.append(f"{gen.OUT.relative_to(ROOT)} is stale — run `make contracts-gen`")


def main() -> int:
    snake_cases()
    table_shape()
    duplicate_refusal()
    on_disk_is_current()

    if failures:
        print(f"test_domaincodes: {len(failures)} failure(s)", file=sys.stderr)
        for f in failures:
            print(f"  - {f}", file=sys.stderr)
        return 1
    print("test_domaincodes OK (shape, uniqueness, drift)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
