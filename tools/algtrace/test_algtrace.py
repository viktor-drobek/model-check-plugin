"""Unit tests for the standalone multi-language algorithm trace CLI."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRIPT = HERE / "algtrace.py"
FIXTURES = HERE / "testdata"


class AlgTraceTest(unittest.TestCase):
    def run_trace(self, *paths):
        return subprocess.run(
            [sys.executable, str(SCRIPT), *map(str, paths)],
            check=False,
            capture_output=True,
            text=True,
        )

    def test_multilanguage_corpus_has_algorithm_and_interaction_edges(self):
        result = self.run_trace(FIXTURES)
        self.assertEqual(result.returncode, 0, result.stderr)
        graph = json.loads(result.stdout)
        self.assertEqual(graph["schema"], "algtrace/v1")
        self.assertEqual(graph["languages"], ["go", "java", "python", "rust"])
        self.assertTrue(graph["nodes"])
        kinds = {edge["kind"] for edge in graph["edges"]}
        self.assertTrue({"sends", "receives", "writes_state", "branches", "loops"}.issubset(kinds))
        self.assertEqual({edge["evidence"] for edge in graph["edges"]}, {"syntactic"})

    def test_explicit_unsupported_file_is_rejected_without_json_output(self):
        result = self.run_trace(HERE / "README.md")
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")
        self.assertIn("unsupported source extension", result.stderr)

    def test_ambiguous_function_name_does_not_create_a_call_edge(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "first.py").write_text("def same():\n    return 1\n", encoding="utf-8")
            (root / "second.py").write_text("def same():\n    return 2\n", encoding="utf-8")
            (root / "caller.py").write_text("def call():\n    same()\n", encoding="utf-8")
            result = self.run_trace(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        graph = json.loads(result.stdout)
        self.assertNotIn("calls", {edge["kind"] for edge in graph["edges"]})


if __name__ == "__main__":
    unittest.main()
