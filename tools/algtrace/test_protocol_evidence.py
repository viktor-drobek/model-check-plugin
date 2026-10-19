"""Tests for offline binary-protocol and standards evidence import."""

import json
import subprocess
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRIPT = HERE / "protocol_evidence.py"
FIXTURES = HERE / "testdata"


class ProtocolEvidenceTest(unittest.TestCase):
    def run_import(self, command, fixture):
        return subprocess.run(
            [sys.executable, str(SCRIPT), command, str(FIXTURES / fixture)],
            check=False,
            capture_output=True,
            text=True,
        )

    def test_tshark_json_becomes_decoded_message_edges(self):
        result = self.run_import("tshark", "sample-tshark.json")
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads(result.stdout)
        self.assertEqual(document["schema"], "protocol-evidence/v1")
        self.assertEqual({edge["evidence"] for edge in document["edges"]}, {"decoded"})
        self.assertEqual({edge["kind"] for edge in document["edges"]}, {"sends", "receives", "decodes_as"})
        self.assertEqual(document["artifact"]["kind"], "tshark-json")

    def test_rfc_bcp14_declaration_marks_uppercase_keywords_as_candidates(self):
        result = self.run_import("rfc", "rfc9999-sample.txt")
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads(result.stdout)
        self.assertEqual(document["document"], {"bcp14_declared": True, "id": "RFC9999"})
        self.assertEqual({item["keyword"] for item in document["requirements"]}, {"MUST", "MUST NOT", "SHOULD", "MAY"})
        self.assertEqual({item["evidence"] for item in document["requirements"]}, {"normative_candidate"})

    def test_ieee_manifest_keeps_only_hash_and_clause_reference(self):
        result = self.run_import("ieee-manifest", "ieee-clause-manifest.json")
        self.assertEqual(result.returncode, 0, result.stderr)
        document = json.loads(result.stdout)
        self.assertEqual(document["document"]["id"], "IEEE 802.1Q-2022")
        self.assertEqual(document["requirements"][0]["evidence"], "licensed_clause_reference")
        self.assertNotIn("pdf", json.dumps(document).lower())


if __name__ == "__main__":
    unittest.main()
