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

    def test_outputs_file(self):
        """An artefact is found by name, and the answer text is irrelevant."""
        check = {"globs": ["*.pml", "*.ir.json"]}
        self.assertFalse(grader.check_outputs_file(check, "here is a proctype", self.outputs, None)[0])
        with open(os.path.join(self.outputs, "model.pml"), "w", encoding="utf-8") as fh:
            fh.write("active proctype p() { skip }\n")
        ok, ev = grader.check_outputs_file(check, "", self.outputs, None)
        self.assertTrue(ok)
        self.assertIn("model.pml", ev)
        self.assertFalse(grader.check_outputs_file({"globs": ["*.json"]}, "", self.outputs, None)[0])
        # nested files count too: a run may put its model in a session directory
        os.makedirs(os.path.join(self.outputs, "session"))
        with open(os.path.join(self.outputs, "session", "m.ir.json"), "w", encoding="utf-8") as fh:
            fh.write("{}")
        self.assertTrue(grader.check_outputs_file({"globs": ["*.ir.json"]}, "", self.outputs, None)[0])

    def test_engine_report(self):
        """The check reads the engine's artefact, and the artefact has to be real.

        Recognising the shape is not enough: a hand-written JSON has the same
        shape. The report is tied to its input by hash, so an assertion about the
        engine having run cannot be satisfied by typing one."""
        model = os.path.join(self.outputs, "m.ir.json")
        with open(model, "w", encoding="utf-8") as fh:
            fh.write('{"schema": "mcd-ir/1"}')
        digest = grader.sha256_of(model)
        report = {
            "engine": {"name": "mcd", "version": "0.1.0", "report_schema": "mcd-report/1"},
            "inputs": [{"kind": "ir", "path": "m.ir.json", "sha256": digest}],
            "properties": [
                {"id": "deadlock", "kind": "deadlock", "status": "verified", "evidence": "exhaustive"},
                {"id": "ctl1", "kind": "ctl", "status": "violated", "evidence": "exhaustive",
                 "temporal": {"logic": "ctl", "fairness": "none"}},
            ],
        }
        answer = "I ran mcd check and everything is verified / exhaustive"
        # No report among the outputs: the prose does not help.
        self.assertFalse(grader.check_engine_report({"type": "engine_report"}, answer, self.outputs, None)[0])
        self.write(os.path.join(self.outputs, "check-1.json"), report)
        self.assertTrue(grader.check_engine_report({"type": "engine_report"}, "", self.outputs, None)[0])
        ok, ev = grader.check_engine_report(
            {"property": "ctl1", "status": "violated", "evidence": "exhaustive"}, "", self.outputs, None)
        self.assertTrue(ok)
        self.assertIn("ctl1", ev)
        # Matching by kind, by a list of kinds, and by the fairness it ran under.
        self.assertTrue(grader.check_engine_report({"kind": "ctl"}, "", self.outputs, None)[0])
        self.assertTrue(grader.check_engine_report({"kind": ["ltl", "ctl"]}, "", self.outputs, None)[0])
        self.assertTrue(grader.check_engine_report({"kind": "ctl", "fairness": "none"}, "", self.outputs, None)[0])
        self.assertFalse(grader.check_engine_report({"kind": "ctl", "fairness": "weak"}, "", self.outputs, None)[0])
        # A bare status must not be answered by a different property: this is the
        # hole the implementation review found (a CTL assertion passing on deadlock).
        self.assertFalse(grader.check_engine_report({"kind": "ltl", "status": "verified"}, "", self.outputs, None)[0])
        # Right property, wrong verdict; and a property that is not there.
        self.assertFalse(grader.check_engine_report({"property": "ctl1", "status": "verified"}, "", self.outputs, None)[0])
        self.assertFalse(grader.check_engine_report({"property": "ltl9"}, "", self.outputs, None)[0])

    def test_engine_report_rejects_a_fabricated_one(self):
        """Every way of writing the JSON by hand that the review demonstrated."""
        base = {
            "engine": {"name": "mcd", "version": "0.1.0", "report_schema": "mcd-report/1"},
            "inputs": [{"kind": "ir", "path": "m.ir.json", "sha256": "0" * 64}],
            "properties": [{"id": "deadlock", "kind": "deadlock", "status": "verified", "evidence": "exhaustive"}],
        }
        want = {"property": "deadlock", "status": "verified"}

        def only(doc):
            for f in os.listdir(self.outputs):
                os.remove(os.path.join(self.outputs, f))
            self.write(os.path.join(self.outputs, "x.json"), doc)
            return grader.check_engine_report(want, "", self.outputs, None)

        # A hash that belongs to no file: nothing was checked.
        self.assertFalse(only(base)[0])
        # A schema that only looks like the engine's.
        bad = json.loads(json.dumps(base))
        bad["engine"]["report_schema"] = "mcd-report/not-a-version"
        self.assertFalse(only(bad)[0])
        # No inputs at all: the report names no model.
        bad = json.loads(json.dumps(base))
        del bad["inputs"]
        self.assertFalse(only(bad)[0])
        # Another engine's report.
        bad = json.loads(json.dumps(base))
        bad["engine"]["name"] = "spin"
        self.assertFalse(only(bad)[0])

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
