"""Unit tests for aggregate.py: python3 -m unittest discover -s evals-workspace
(run from model-check-plugin/)."""

import json
import os
import shutil
import tempfile
import unittest

import aggregate


def grading(flags):
    return {"expectations": [{"text": "a%d" % i, "passed": f, "evidence": "e"} for i, f in enumerate(flags)]}


class AggregateTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        # eval 3 written before eval 1 on purpose: order must come from the id
        self.make_eval(3, "petri", with_skill=[True, True], without=[True, False],
                       timing={"with_skill": {"tokens": 100, "duration_s": 10.0, "tool_uses": 4},
                               "without_skill": {"tokens": 40, "duration_s": 4.0, "tool_uses": 1}})
        self.make_eval(1, "mutex", with_skill=[True, True, True, True], without=[False, False, True, False],
                       timing={"with_skill": {"tokens": 200, "duration_s": 30.0, "tool_uses": 8},
                               "without_skill": {"tokens": 60, "duration_s": 6.0, "tool_uses": 2}})

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def make_eval(self, eval_id, name, with_skill, without, timing):
        d = os.path.join(self.tmp, "eval-%d-%s" % (eval_id, name))
        for conf, flags in (("with_skill", with_skill), ("without_skill", without)):
            os.makedirs(os.path.join(d, conf, "outputs"))
            with open(os.path.join(d, conf, "grading.json"), "w") as fh:
                json.dump(grading(flags), fh)
        with open(os.path.join(d, "eval_metadata.json"), "w") as fh:
            json.dump({"eval_id": eval_id, "eval_name": name, "prompt": "p", "date": "2026-09-25"}, fh)
        with open(os.path.join(d, "timing.json"), "w") as fh:
            json.dump(timing, fh)

    def test_stats(self):
        self.assertEqual(aggregate.calculate_stats([]), {"mean": 0.0, "stddev": 0.0, "min": 0.0, "max": 0.0})
        self.assertEqual(aggregate.calculate_stats([1.0]), {"mean": 1.0, "stddev": 0.0, "min": 1.0, "max": 1.0})
        s = aggregate.calculate_stats([1.0, 3.0])
        self.assertEqual((s["mean"], s["min"], s["max"]), (2.0, 1.0, 3.0))
        self.assertAlmostEqual(s["stddev"], 1.4142, places=4)

    def test_runs_ordered_with_skill_first_then_by_eval_id(self):
        runs = aggregate.load_runs(self.tmp)
        self.assertEqual([(r["configuration"], r["eval_id"]) for r in runs],
                         [("with_skill", 1), ("with_skill", 3), ("without_skill", 1), ("without_skill", 3)])
        self.assertEqual(runs[0]["result"], {"pass_rate": 1.0, "passed": 4, "failed": 0, "total": 4,
                                             "time_seconds": 30.0, "tokens": 200, "tool_calls": 8, "errors": 0})
        self.assertEqual(runs[2]["result"]["pass_rate"], 0.25)
        self.assertEqual(runs[0]["expectations"][0], {"text": "a0", "passed": True, "evidence": "e"})

    def test_summary_order_and_delta(self):
        b = aggregate.build_benchmark(self.tmp, "s", "p", "m", None)
        self.assertEqual(list(b["run_summary"].keys()), ["with_skill", "without_skill", "delta"])
        self.assertEqual(b["run_summary"]["with_skill"]["pass_rate"]["mean"], 1.0)
        self.assertEqual(b["run_summary"]["without_skill"]["pass_rate"]["mean"], 0.375)
        self.assertEqual(b["run_summary"]["delta"], {"pass_rate": "+0.62", "time_seconds": "+15.0", "tokens": "+100"})
        self.assertEqual(b["metadata"]["evals_run"], [1, 3])
        self.assertEqual(b["metadata"]["timestamp"], "2026-09-25T00:00:00Z")
        self.assertEqual(b["metadata"]["runs_per_configuration"], 1)

    def test_notes_name_undifferentiating_assertions(self):
        b = aggregate.build_benchmark(self.tmp, "s", "p", "m", "T")
        notes = "\n".join(b["notes"])
        self.assertIn("Eval 1 (mutex): with_skill 4/4, without_skill 1/4", notes)
        self.assertIn("Eval 1: 1 of 4 assertions pass in both configurations", notes)
        self.assertIn("'a2'", notes)
        self.assertIn("Eval 3: 1 of 2 assertions pass in both configurations", notes)
        self.assertNotIn("failed:", notes)

    def test_notes_name_skill_failures(self):
        self.make_eval(5, "ccode", with_skill=[True, False], without=[False, False],
                       timing={"with_skill": {"tokens": 1, "duration_s": 1}, "without_skill": {"tokens": 1, "duration_s": 1}})
        b = aggregate.build_benchmark(self.tmp, "s", "p", "m", "T")
        self.assertIn("Eval 5: the with_skill run failed: 'a1'", b["notes"])

    def test_missing_configuration_is_skipped(self):
        shutil.rmtree(os.path.join(self.tmp, "eval-3-petri", "without_skill"))
        runs = aggregate.load_runs(self.tmp)
        self.assertEqual([(r["configuration"], r["eval_id"]) for r in runs],
                         [("with_skill", 1), ("with_skill", 3), ("without_skill", 1)])

    def test_main_writes_json_and_markdown(self):
        out = os.path.join(self.tmp, "bench", "benchmark.json")
        os.makedirs(os.path.dirname(out))
        self.assertEqual(aggregate.main([self.tmp, "-o", out, "--timestamp", "T", "--executor-model", "M"]), 0)
        with open(out) as fh:
            b = json.load(fh)
        self.assertEqual([r["configuration"] for r in b["runs"]], ["with_skill", "with_skill", "without_skill", "without_skill"])
        with open(os.path.join(self.tmp, "bench", "benchmark.md")) as fh:
            md = fh.read()
        self.assertIn("| Metric | With Skill | Without Skill | Delta |", md)
        self.assertIn("| Pass Rate | 100% ± 0% | 38% ± 18% | +0.62 |", md)
        self.assertIn("| 1 mutex | with_skill | 4/4 | 100% | 30.0 | 200 | 8 |", md)
        self.assertIn("**Model**: M", md)


if __name__ == "__main__":
    unittest.main()
