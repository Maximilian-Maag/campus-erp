#!/usr/bin/env python3
"""Tests for the ubiquitous-language generator and checker."""

import importlib.util
import json
import sys
import tempfile
import unittest
from unittest import mock
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / "tools" / f"{name}.py")
    assert spec is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


language = load("gen_language")
checker = load("check_language")


class CheckerTest(unittest.TestCase):
    def setUp(self):
        self.data = json.loads((ROOT / "docs" / "ubiquitous-language.json").read_text())

    def test_accepts_the_canonical_spelling(self):
        findings = checker.scan("docs/x.md", "A cost centre and its budget centre.", "prose", self.data)
        self.assertEqual(findings, [])

    def test_rejects_the_other_spelling_in_prose(self):
        findings = checker.scan("docs/x.md", "A cost center and its budget center.", "prose", self.data)
        self.assertEqual(len(findings), 2)

    def test_rejects_a_banned_synonym(self):
        findings = checker.scan("docs/x.md", "The Zahlungsanordnung is approved.", "prose", self.data)
        self.assertEqual(len(findings), 1)
        self.assertIn("payment order", findings[0])

    def test_ignores_code_spans_and_fences(self):
        text = "The endpoint `/api/cost-centers` is a URL.\n\n```\ncost center\n```\n"
        findings = checker.scan("docs/x.md", text, "prose", self.data)
        self.assertEqual(findings, [])

    def test_history_and_generated_are_not_scanned(self):
        self.assertIsNone(checker.classify("backend/internal/platform/db/migrations/0002_old.sql"))
        self.assertIsNone(checker.classify("frontend/src/app/core/api/generated/models.ts"))
        self.assertEqual(checker.classify("backend/internal/modules/klr/klr.go"), "code")
        self.assertEqual(checker.classify("docs/10-budget.md"), "prose")


class GeneratorTest(unittest.TestCase):
    def test_markdown_is_current(self):
        data = json.loads((ROOT / "docs" / "ubiquitous-language.json").read_text())
        rendered = language.render(data)
        self.assertEqual(rendered, (ROOT / "docs" / "ubiquitous-language.md").read_text())

    def test_renders_every_concept(self):
        data = json.loads((ROOT / "docs" / "ubiquitous-language.json").read_text())
        rendered = language.render(data)
        for term in data["terms"]:
            self.assertIn(term["concept"], rendered)

    def test_render_is_deterministic(self):
        data = json.loads((ROOT / "docs" / "ubiquitous-language.json").read_text())
        self.assertEqual(language.render(data), language.render(data))

    def test_check_detects_drift(self):
        """--check exits 1 when the markdown no longer matches the JSON."""
        data = json.loads((ROOT / "docs" / "ubiquitous-language.json").read_text())
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "ubiquitous-language.md"
            original = getattr(language, "TARGET")
            setattr(language, "TARGET", target)
            try:
                target.write_text("stale\n", encoding="utf-8")
                with mock.patch.object(sys, "argv", ["gen_language.py", "--check"]):
                    self.assertEqual(language.main(), 1)
                target.write_text(language.render(data), encoding="utf-8")
                with mock.patch.object(sys, "argv", ["gen_language.py", "--check"]):
                    self.assertEqual(language.main(), 0)
            finally:
                setattr(language, "TARGET", original)


if __name__ == "__main__":
    unittest.main()
