#!/usr/bin/env python3
"""Unit tests for trigger_measure.py (protocol point 3)."""

import json
import os
import tempfile
import unittest

import trigger_measure as tm

HERE = os.path.dirname(os.path.abspath(__file__))
EVALSET = os.path.join(HERE, "trigger-eval.json")


def q(i, trig):
    return {"id": i, "query": f"query {i}", "should_trigger": trig}


SAMPLE = [q(f"t{n:02d}", True) for n in range(1, 11)] + \
         [q(f"n{n:02d}", False) for n in range(1, 11)]


class TestLoad(unittest.TestCase):
    def test_real_set_loads_and_has_both_classes(self):
        qs = tm.load_queries(EVALSET)
        self.assertGreaterEqual(len(qs), 16)
        self.assertGreaterEqual(sum(1 for x in qs if x["should_trigger"]), 8)
        self.assertGreaterEqual(sum(1 for x in qs if not x["should_trigger"]), 8)

    def test_duplicate_query_is_rejected(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as fh:
            json.dump([q("a", True), {"id": "b", "query": "query a",
                                      "should_trigger": False}], fh)
            path = fh.name
        with self.assertRaises(ValueError):
            tm.load_queries(path)
        os.unlink(path)

    def test_non_boolean_label_is_rejected(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as fh:
            json.dump([{"id": "a", "query": "x", "should_trigger": "yes"}], fh)
            path = fh.name
        with self.assertRaises(ValueError):
            tm.load_queries(path)
        os.unlink(path)


class TestSplit(unittest.TestCase):
    def test_deterministic_in_the_seed(self):
        self.assertEqual(tm.split(SAMPLE, 7), tm.split(SAMPLE, 7))
        self.assertNotEqual(tm.split(SAMPLE, 7), tm.split(SAMPLE, 8))

    def test_partitions_every_query_exactly_once(self):
        train, test = tm.split(SAMPLE)
        self.assertEqual(sorted(train + test), sorted(x["id"] for x in SAMPLE))
        self.assertEqual(set(train) & set(test), set())

    def test_both_classes_on_both_sides(self):
        train, test = tm.split(SAMPLE)
        for side in (train, test):
            self.assertTrue(any(i.startswith("t") for i in side))
            self.assertTrue(any(i.startswith("n") for i in side))

    def test_sizes_follow_the_fraction(self):
        train, test = tm.split(SAMPLE, train_frac=0.6)
        self.assertEqual((len(train), len(test)), (12, 8))

    def test_real_set_splits_with_both_classes_held_out(self):
        qs = tm.load_queries(EVALSET)
        train, test = tm.split(qs)
        want = {x["id"]: x["should_trigger"] for x in qs}
        self.assertTrue(any(want[i] for i in test))
        self.assertTrue(any(not want[i] for i in test))


class TestMajority(unittest.TestCase):
    def test_two_of_three_wins(self):
        runs = [{"a": "yes"}, {"a": "yes"}, {"a": "no"}]
        self.assertEqual(tm.majority(runs), {"a": True})

    def test_tie_counts_as_no_trigger(self):
        runs = [{"a": "yes"}, {"a": "no"}]
        self.assertEqual(tm.majority(runs), {"a": False})

    def test_missing_id_does_not_vote(self):
        runs = [{"a": "yes"}, {}, {"a": "yes"}]
        self.assertEqual(tm.majority(runs), {"a": True})


class TestScore(unittest.TestCase):
    def test_perfect(self):
        dec = {x["id"]: x["should_trigger"] for x in SAMPLE}
        s = tm.score(dec, SAMPLE, [x["id"] for x in SAMPLE])
        self.assertEqual(s["accuracy"], 1.0)
        self.assertEqual((s["false_positive"], s["false_negative"]), (0, 0))

    def test_confusion_counts(self):
        dec = {x["id"]: x["should_trigger"] for x in SAMPLE}
        dec["t01"] = False          # a miss
        dec["n01"] = True           # a false trigger
        s = tm.score(dec, SAMPLE, [x["id"] for x in SAMPLE])
        self.assertEqual(s["false_negative"], 1)
        self.assertEqual(s["false_positive"], 1)
        self.assertEqual(s["accuracy"], 0.9)

    def test_scores_only_the_ids_asked_for(self):
        dec = {x["id"]: x["should_trigger"] for x in SAMPLE}
        dec["t01"] = False
        train, test = tm.split(SAMPLE)
        s_train = tm.score(dec, SAMPLE, train)
        s_test = tm.score(dec, SAMPLE, test)
        self.assertEqual(s_train["n"] + s_test["n"], len(SAMPLE))
        # The one error lands on exactly one side, never on both.
        self.assertEqual(s_train["false_negative"] + s_test["false_negative"], 1)

    def test_missing_decision_is_reported_not_counted(self):
        dec = {x["id"]: x["should_trigger"] for x in SAMPLE if x["id"] != "t01"}
        s = tm.score(dec, SAMPLE, [x["id"] for x in SAMPLE])
        self.assertEqual(s["missing"], ["t01"])
        self.assertEqual(s["n"], len(SAMPLE) - 1)


class TestPrompt(unittest.TestCase):
    def test_prompt_hides_the_labels_and_names_every_query(self):
        qs = tm.load_queries(EVALSET)
        order, text = tm.judge_prompt(qs, "DESCRIPTION UNDER TEST", "/tmp/out.json")
        self.assertNotIn("should_trigger", text)
        self.assertNotIn("why", text.split("## Requests")[1])
        for x in qs:
            self.assertIn(x["query"], text)
        self.assertIn("DESCRIPTION UNDER TEST", text)
        self.assertEqual(sorted(order), sorted(x["id"] for x in qs))

    def test_order_is_the_same_for_every_variant_and_interleaves_classes(self):
        qs = tm.load_queries(EVALSET)
        o1, _ = tm.judge_prompt(qs, "A", "/tmp/a.json")
        o2, _ = tm.judge_prompt(qs, "B", "/tmp/b.json")
        self.assertEqual(o1, o2)
        self.assertNotEqual(o1, sorted(o1))

    def test_roster_carries_decoys(self):
        text = tm.roster_text("D")
        self.assertIn("model-check", text)
        self.assertIn("code-review", text)
        self.assertGreaterEqual(len(text.splitlines()), 10)




class TestChoose(unittest.TestCase):
    def _v(self, name, test_acc, train_acc=0.0):
        return {"variant": name, "test_accuracy": test_acc, "train_accuracy": train_acc}

    def test_best_held_out_wins(self):
        vs = [self._v("a", 0.75), self._v("b", 1.0), self._v("c", 0.5)]
        self.assertEqual(tm.choose(vs)["variant"], "b")

    def test_tie_keeps_the_incumbent_named_first(self):
        vs = [self._v("incumbent", 1.0), self._v("challenger", 1.0)]
        self.assertEqual(tm.choose(vs)["variant"], "incumbent")

    def test_a_better_training_score_does_not_win(self):
        vs = [self._v("a", 0.9, train_acc=0.5), self._v("b", 0.8, train_acc=1.0)]
        self.assertEqual(tm.choose(vs)["variant"], "a")


if __name__ == "__main__":
    unittest.main()
