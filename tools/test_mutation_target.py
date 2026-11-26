#!/usr/bin/env python3
"""Tests for the mutation target's time handling.

The gate is only meaningful when a shard's bound is measured against a warm
build. A cold cache makes the first run's duration the build time, so every
mutant gets minutes and the shard takes the job's whole budget; a warm cache at
a coefficient of 3 makes every mutant time out because the suite is milliseconds.
These checks pin the two properties that keep the bound honest:

  * the build cache is warmed before gremlins measures its baseline;
  * the shard's output is not piped through tee, because a killed gremlins can
    leave test processes holding the pipe and the step never ends.
"""

import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MAKEFILE = ROOT / "Makefile"


def target_body(name: str) -> str:
    """The recipe lines of a Makefile target, up to the next target."""
    text = MAKEFILE.read_text(encoding="utf-8")
    match = re.search(rf"^{re.escape(name)}:.*$", text, re.MULTILINE)
    if not match:
        raise AssertionError(f"{name} is not a target")
    rest = text[match.end():]
    lines = []
    for line in rest.splitlines():
        if line and not line.startswith("\t") and not line.startswith(" "):
            break
        lines.append(line)
    return "\n".join(lines)


class MutationTargetTest(unittest.TestCase):
    def test_the_build_cache_is_warmed_before_gremlins(self):
        body = target_body("backend-mutation")
        warm = body.find("test -run")
        gremlins = body.find("gremlins unleash")
        self.assertNotEqual(warm, -1, "backend-mutation does not warm the build cache")
        self.assertNotEqual(gremlins, -1, "backend-mutation does not run gremlins")
        self.assertLess(warm, gremlins, "the warm-up must run before gremlins")

    def test_the_shard_output_is_not_piped_through_tee(self):
        body = target_body("backend-mutation")
        self.assertNotIn("| tee", body, "tee can wait on processes a timeout killed")

    def test_the_timeout_coefficient_leaves_room_for_a_warm_suite(self):
        text = MAKEFILE.read_text(encoding="utf-8")
        match = re.search(r"^MUTATION_TIMEOUT_COEFFICIENT\s*\?=\s*(\d+)", text, re.MULTILINE)
        assert match is not None, "MUTATION_TIMEOUT_COEFFICIENT is not a target variable"
        self.assertGreaterEqual(
            int(match.group(1)),
            10,
            "a coefficient below 10 times out a warm suite whose runtime is milliseconds",
        )


if __name__ == "__main__":
    unittest.main()
