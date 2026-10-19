#!/usr/bin/env python3
"""Offline tests for release source admission."""

import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("validate-release.py")


class ReleaseValidationTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location("validate_release", SCRIPT)
        self.validator = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.validator)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "--quiet")
        self.git("config", "user.name", "Release Test")
        self.git("config", "user.email", "release@example.invalid")
        self.manifest("plugin.json", "0.4.0")
        self.manifest(".claude-plugin/plugin.json", "0.4.0")
        self.git("add", ".")
        self.git("commit", "--quiet", "-m", "fixture")
        self.git("tag", "v0.4.0")

    def git(self, *args):
        return subprocess.check_output(
            ["git", "-C", str(self.root), *args], text=True, timeout=10
        ).strip()

    def manifest(self, name, version):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps({"version": version}), encoding="utf-8")

    def test_accepts_matching_lightweight_tag(self):
        result = self.validator.validate(self.root, "v0.4.0")
        self.assertEqual(result, {
            "tag": "v0.4.0", "version": "0.4.0",
            "source_commit": self.git("rev-parse", "HEAD"),
        })

    def test_accepts_annotated_tag_without_v(self):
        self.git("tag", "-a", "0.4.0", "-m", "release")
        self.assertEqual(self.validator.validate(self.root, "0.4.0")["version"], "0.4.0")

    def test_rejects_wrong_version(self):
        self.git("tag", "v0.5.0")
        with self.assertRaisesRegex(ValueError, "release tag does not match plugin version"):
            self.validator.validate(self.root, "v0.5.0")

    def test_rejects_disagreeing_manifests(self):
        self.manifest(".claude-plugin/plugin.json", "0.5.0")
        with self.assertRaisesRegex(ValueError, "manifest versions differ"):
            self.validator.validate(self.root, "v0.4.0")

    def test_rejects_non_tag_input(self):
        for tag in ("", "main", "--help", "v0.4.0\nINJECT=value", "v0.4.0^{commit}"):
            with self.subTest(tag=tag), self.assertRaisesRegex(ValueError, "invalid release tag"):
                self.validator.validate(self.root, tag)

    def test_rejects_missing_tag(self):
        with self.assertRaisesRegex(ValueError, "git release validation failed"):
            self.validator.validate(self.root, "v0.4.1")

    def test_rejects_checkout_of_different_commit(self):
        (self.root / "extra").write_text("new commit", encoding="utf-8")
        self.git("add", ".")
        self.git("commit", "--quiet", "-m", "another commit")
        with self.assertRaisesRegex(ValueError, "checkout does not match release tag"):
            self.validator.validate(self.root, "v0.4.0")

    def test_rejects_dirty_tracked_source(self):
        (self.root / "plugin.json").write_text('{"version": "0.4.0"}\n', encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "release checkout is dirty"):
            self.validator.validate(self.root, "v0.4.0")


if __name__ == "__main__":
    unittest.main()
