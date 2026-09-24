"""Unit tests for grader.py: python3 -m unittest evals-workspace/test_grader.py
(run from model-check-plugin/, or `python3 -m unittest discover evals-workspace`)."""

import json
import os
import shutil
import tempfile
import unittest

import grader

NET = {
    "name": "n",
    "places": [{"name": "p1", "initial": 1}, {"name": "p2"}],
    "transitions": [{"name": "t1", "inputs": [{"place": "p1"}], "outputs": [{"place": "p2"}]}],
}


class GraderTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.outputs = os.path.join(self.tmp, "outputs")
        self.skill = os.path.join(self.tmp, "skill")
        os.makedirs(os.path.join(self.skill, "evals", "fixtures"))
        os.makedirs(self.outputs)
        self.write(os.path.join(self.skill, "evals", "fixtures", "net.json"), NET)

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def write(self, path, doc):
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(doc, fh)

    # --- text checks -----------------------------------------------------
    def test_regex_and_not_regex(self):
        ok, ev = grader.check_regex({"pattern": r"\bviolated\b"}, "status violated", None, None)
        self.assertTrue(ok)
        self.assertIn("violated", ev)
        ok, _ = grader.check_regex({"pattern": r"\bviolated\b"}, "not violate d", None, None)
        self.assertFalse(ok)
        ok, ev = grader.check_not_regex({"pattern": r"(?i)\bno errors\b"}, "there are No Errors", None, None)
        self.assertFalse(ok)
        self.assertIn("No Errors", ev)
        ok, _ = grader.check_not_regex({"pattern": r"\bproved\b"}, "holds on the model", None, None)
        self.assertTrue(ok)

    def test_regex_order(self):
        check = {"patterns": ["without fairness", "with weak fairness"]}
        self.assertTrue(grader.check_regex_order(check, "without fairness: violated; with weak fairness: verified", None, None)[0])
        self.assertFalse(grader.check_regex_order(check, "with weak fairness: verified; without fairness: violated", None, None)[0])
        self.assertFalse(grader.check_regex_order(check, "without fairness only", None, None)[0])

    # --- petri shape -----------------------------------------------------
    def test_petri_shape_accepts_schema_shape_and_defaults(self):
        self.assertIsNone(grader.petri_shape_error(NET))
        full = json.loads(json.dumps(NET))
        full["places"][0]["capacity"] = 3
        full["transitions"][0]["inputs"][0]["weight"] = 1
        full["transitions"][0]["line"] = 4
        self.assertIsNone(grader.petri_shape_error(full))

    def test_petri_shape_rejects_sketch_keys_and_bad_values(self):
        bad = {"places": [{"id": "p1", "initial": 1}], "transitions": [{"id": "t1", "inputs": [], "outputs": []}]}
        self.assertIn("places[0]", grader.petri_shape_error(bad))
        mult = json.loads(json.dumps(NET))
        mult["transitions"][0]["inputs"][0] = {"place": "p1", "multiplicity": 1}
        self.assertIn("inputs[0]", grader.petri_shape_error(mult))
        over = json.loads(json.dumps(NET))
        over["places"][0]["initial"] = 300
        self.assertIn("initial", grader.petri_shape_error(over))
        inh = json.loads(json.dumps(NET))
        inh["transitions"][0]["inputs"][0]["inhibitor"] = True
        self.assertIn("inhibitor", grader.petri_shape_error(inh))
        unknown_place = json.loads(json.dumps(NET))
        unknown_place["transitions"][0]["outputs"][0]["place"] = "zz"
        self.assertIn("unknown", grader.petri_shape_error(unknown_place))
        self.assertIn("non-empty", grader.petri_shape_error({"places": [], "transitions": []}))

    def test_normalize_ignores_arc_order_and_explicit_defaults(self):
        a = json.loads(json.dumps(NET))
        b = {
            "places": [{"name": "p1", "initial": 1, "capacity": 255}, {"name": "p2", "initial": 0}],
            "transitions": [{"name": "t1", "inputs": [{"place": "p1", "weight": 1}], "outputs": [{"place": "p2", "weight": 1}]}],
        }
        self.assertEqual(grader.normalize_net(a), grader.normalize_net(b))
        c = json.loads(json.dumps(NET))
        c["places"][1]["initial"] = 1
        self.assertNotEqual(grader.normalize_net(a), grader.normalize_net(c))

    def test_petri_json_valid_search_and_equivalence(self):
        check = {"type": "petri_json_valid", "equivalent_to": "evals/fixtures/net.json"}
        ok, ev = grader.check_petri_json_valid(check, "", self.outputs, self.skill)
        self.assertFalse(ok)
        self.assertIn("no *.json", ev)
        # a report-like JSON is skipped, the net in a subdirectory is found
        self.write(os.path.join(self.outputs, "report.json"), {"properties": [{"status": "violated"}]})
        sub = os.path.join(self.outputs, "session")
        os.makedirs(sub)
        different = json.loads(json.dumps(NET))
        different["places"][1]["initial"] = 2
        self.write(os.path.join(sub, "other.json"), different)
        ok, ev = grader.check_petri_json_valid(check, "", self.outputs, self.skill)
        self.assertFalse(ok)
        self.assertIn("not equivalent", ev)
        self.write(os.path.join(sub, "net.json"), NET)
        ok, ev = grader.check_petri_json_valid(check, "", self.outputs, self.skill)
        self.assertTrue(ok)
        self.assertIn("net.json", ev)
        # without equivalent_to any valid net passes
        ok, _ = grader.check_petri_json_valid({"type": "petri_json_valid"}, "", self.outputs, self.skill)
        self.assertTrue(ok)

    # --- json_field ------------------------------------------------------
    def test_json_field(self):
        self.write(os.path.join(self.outputs, "run.report.json"), {"properties": [{"id": "deadlock", "status": "violated"}]})
        check = {"file_glob": "*.report.json", "path": "properties.0.status", "equals": "violated"}
        self.assertTrue(grader.check_json_field(check, "", self.outputs, None)[0])
        check["equals"] = "verified"
        self.assertFalse(grader.check_json_field(check, "", self.outputs, None)[0])
        check["path"] = "properties.5.status"
        self.assertFalse(grader.check_json_field(check, "", self.outputs, None)[0])

    # --- grade -----------------------------------------------------------
    def test_grade_shapes_grading_json(self):
        entry = {
            "id": 3,
            "name": "x",
            "assertions": [
                {"text": "a", "check": {"type": "regex", "pattern": "t1"}},
                {"text": "b", "check": {"type": "not_regex", "pattern": "proved"}},
                {"text": "c", "check": {"type": "bogus"}},
            ],
        }
        res = grader.grade(entry, "t1, t4 proved", self.outputs, self.skill, run="with_skill")
        self.assertEqual(res["run"], "with_skill")
        self.assertEqual(res["total"], 3)
        self.assertEqual(res["passed_count"], 1)
        self.assertEqual([e["passed"] for e in res["expectations"]], [True, False, False])
        for e in res["expectations"]:
            self.assertEqual(set(e), {"text", "passed", "evidence"})
        self.assertIn("unknown check type", res["expectations"][2]["evidence"])

    def test_main_writes_file(self):
        evals = os.path.join(self.tmp, "evals.json")
        self.write(evals, {"evals": [{"id": 1, "name": "e", "assertions": [{"text": "a", "check": {"type": "regex", "pattern": "ok"}}]}]})
        answer = os.path.join(self.tmp, "answer.md")
        with open(answer, "w", encoding="utf-8") as fh:
            fh.write("all ok")
        out = os.path.join(self.tmp, "grading.json")
        grader.main(["--evals", evals, "--id", "1", "--answer", answer, "--outputs", self.outputs,
                     "--skill-dir", self.skill, "--out", out, "--run", "r"])
        with open(out, encoding="utf-8") as fh:
            doc = json.load(fh)
        self.assertEqual(doc["passed_count"], 1)
        self.assertEqual(doc["expectations"][0]["passed"], True)


if __name__ == "__main__":
    unittest.main()
