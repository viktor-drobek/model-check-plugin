#!/usr/bin/env python3
"""Grader for the model-check skill evals (plan 14 §8.2).

Reads one eval from skills/model-check/evals/evals.json, applies every
assertion's `check` to a run's answer text and outputs directory, and writes
grading.json in the shape {"expectations": [{"text", "passed", "evidence"}]}
(the key `expectations` belongs to grading.json only; the eval file says
`assertions`).

Check types (all objective — no judgement, no model call):

  regex            {"pattern"}            re.search over the answer text
  not_regex        {"pattern"}            no match over the answer text
  regex_order      {"patterns": [...]}    every pattern matches and the first
                                          matches appear in the given order
  petri_json_valid {"equivalent_to"?}     some *.json under the outputs
                                          directory is a place/transition net
                                          in the engine's schema shape and,
                                          when equivalent_to is set, has the
                                          same places, initial marking,
                                          transitions and arcs as that fixture
                                          (defaults applied: initial 0,
                                          weight 1, capacity 255)
  json_field       {"file_glob", "path",  some file matching file_glob under
                    "equals"}             outputs has the value at the dotted
                                          path

Usage:
  grader.py --evals EVALS_JSON --id N --answer ANSWER_FILE
            --outputs DIR --skill-dir SKILL_DIR --out grading.json [--run NAME]
"""

import argparse
import fnmatch
import json
import os
import re
import sys

PLACE_KEYS = {"name", "initial", "capacity", "line"}
TRANSITION_KEYS = {"name", "inputs", "outputs", "line"}
ARC_KEYS = {"place", "weight", "inhibitor"}
IDENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def check_regex(check, answer, _outputs, _skill_dir):
    m = re.search(check["pattern"], answer)
    if m:
        return True, "matched %r at %d" % (m.group(0)[:80], m.start())
    return False, "pattern %r not found" % check["pattern"]


def check_not_regex(check, answer, _outputs, _skill_dir):
    m = re.search(check["pattern"], answer)
    if m:
        return False, "forbidden match %r at %d" % (m.group(0)[:80], m.start())
    return True, "pattern %r absent" % check["pattern"]


def check_regex_order(check, answer, _outputs, _skill_dir):
    positions = []
    for p in check["patterns"]:
        m = re.search(p, answer)
        if not m:
            return False, "pattern %r not found" % p
        positions.append((m.start(), m.group(0)[:40]))
    for (a, _), (b, _) in zip(positions, positions[1:]):
        if b <= a:
            return False, "patterns out of order: %s" % positions
    return True, "in order: %s" % positions


def petri_shape_error(doc):
    """Return None when doc has the shape of the engine's Petri schema, else a
    message. Mirrors frontend/petri/schema.json clause by clause for the checks
    a grader can do without the engine."""
    if not isinstance(doc, dict):
        return "not an object"
    extra = set(doc) - {"name", "places", "transitions"}
    if extra:
        return "unknown top-level keys %s" % sorted(extra)
    for key in ("places", "transitions"):
        if not isinstance(doc.get(key), list) or not doc[key]:
            return "%s must be a non-empty array" % key
    names = set()
    for i, p in enumerate(doc["places"]):
        if not isinstance(p, dict) or set(p) - PLACE_KEYS or "name" not in p:
            return "places[%d] has unknown or missing keys" % i
        if not IDENT.match(str(p["name"])) or p["name"] in names:
            return "places[%d].name invalid or duplicate" % i
        names.add(p["name"])
        cap = p.get("capacity", 255)
        init = p.get("initial", 0)
        if not isinstance(cap, int) or not 1 <= cap <= 255:
            return "places[%d].capacity out of 1..255" % i
        if not isinstance(init, int) or init < 0 or init > cap:
            return "places[%d].initial invalid" % i
    tnames = set()
    for i, t in enumerate(doc["transitions"]):
        if not isinstance(t, dict) or set(t) - TRANSITION_KEYS or "name" not in t:
            return "transitions[%d] has unknown or missing keys" % i
        if not IDENT.match(str(t["name"])) or t["name"] in tnames or t["name"] in names:
            return "transitions[%d].name invalid or duplicate" % i
        tnames.add(t["name"])
        for side in ("inputs", "outputs"):
            for j, a in enumerate(t.get(side, [])):
                if not isinstance(a, dict) or set(a) - ARC_KEYS or "place" not in a:
                    return "transitions[%d].%s[%d] malformed" % (i, side, j)
                if a["place"] not in names:
                    return "transitions[%d].%s[%d].place unknown" % (i, side, j)
                w = a.get("weight", 1)
                if not isinstance(w, int) or w < 1:
                    return "transitions[%d].%s[%d].weight < 1" % (i, side, j)
                if a.get("inhibitor", False):
                    return "transitions[%d].%s[%d] is an inhibitor arc (rejected by the engine)" % (i, side, j)
    return None


def normalize_net(doc):
    """Structure the comparison ignores order of arcs and JSON formatting but
    keeps names, initial marking, capacities and weights."""
    places = {p["name"]: (p.get("initial", 0), p.get("capacity", 255)) for p in doc["places"]}
    transitions = {}
    for t in doc["transitions"]:
        ins = tuple(sorted((a["place"], a.get("weight", 1)) for a in t.get("inputs", [])))
        outs = tuple(sorted((a["place"], a.get("weight", 1)) for a in t.get("outputs", [])))
        transitions[t["name"]] = (ins, outs)
    return places, transitions


def json_files(outputs):
    for root, _dirs, files in os.walk(outputs):
        for f in sorted(files):
            if f.endswith(".json"):
                yield os.path.join(root, f)


def check_petri_json_valid(check, _answer, outputs, skill_dir):
    reference = None
    if check.get("equivalent_to"):
        with open(os.path.join(skill_dir, check["equivalent_to"]), encoding="utf-8") as fh:
            reference = normalize_net(json.load(fh))
    reasons = []
    for path in json_files(outputs):
        try:
            with open(path, encoding="utf-8") as fh:
                doc = json.load(fh)
        except (OSError, ValueError) as e:
            reasons.append("%s: %s" % (path, e))
            continue
        err = petri_shape_error(doc)
        if err:
            reasons.append("%s: %s" % (os.path.relpath(path, outputs), err))
            continue
        if reference is not None and normalize_net(doc) != reference:
            reasons.append("%s: valid net but not equivalent to %s" % (os.path.relpath(path, outputs), check["equivalent_to"]))
            continue
        return True, "%s is a valid net%s" % (
            os.path.relpath(path, outputs),
            " equivalent to " + check["equivalent_to"] if reference is not None else "")
    return False, "no valid net under outputs: " + ("; ".join(reasons) if reasons else "no *.json files")


def get_path(doc, dotted):
    cur = doc
    for part in dotted.split("."):
        if isinstance(cur, list):
            try:
                cur = cur[int(part)]
            except (ValueError, IndexError):
                return None, False
        elif isinstance(cur, dict) and part in cur:
            cur = cur[part]
        else:
            return None, False
    return cur, True


def check_json_field(check, _answer, outputs, _skill_dir):
    for path in json_files(outputs):
        if not fnmatch.fnmatch(os.path.basename(path), check.get("file_glob", "*.json")):
            continue
        try:
            with open(path, encoding="utf-8") as fh:
                doc = json.load(fh)
        except (OSError, ValueError):
            continue
        val, ok = get_path(doc, check["path"])
        if ok and val == check["equals"]:
            return True, "%s: %s == %r" % (os.path.relpath(path, outputs), check["path"], val)
    return False, "no file matching %s has %s == %r" % (check.get("file_glob", "*.json"), check["path"], check["equals"])


CHECKS = {
    "regex": check_regex,
    "not_regex": check_not_regex,
    "regex_order": check_regex_order,
    "petri_json_valid": check_petri_json_valid,
    "json_field": check_json_field,
}


def grade(eval_entry, answer, outputs, skill_dir, run=None):
    expectations = []
    for a in eval_entry["assertions"]:
        check = a["check"]
        fn = CHECKS.get(check.get("type"))
        if fn is None:
            passed, evidence = False, "unknown check type %r" % check.get("type")
        else:
            passed, evidence = fn(check, answer, outputs, skill_dir)
        expectations.append({"text": a["text"], "passed": bool(passed), "evidence": evidence})
    passed_count = sum(1 for e in expectations if e["passed"])
    return {
        "eval_id": eval_entry["id"],
        "eval_name": eval_entry.get("name"),
        "run": run,
        "passed_count": passed_count,
        "total": len(expectations),
        "expectations": expectations,
    }


def load_eval(evals_path, eval_id):
    with open(evals_path, encoding="utf-8") as fh:
        data = json.load(fh)
    for e in data["evals"]:
        if e["id"] == eval_id:
            return e
    raise SystemExit("no eval with id %d in %s" % (eval_id, evals_path))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--evals", required=True)
    ap.add_argument("--id", type=int, required=True)
    ap.add_argument("--answer", required=True, help="file with the run's final answer text")
    ap.add_argument("--outputs", required=True, help="directory with the run's artefacts")
    ap.add_argument("--skill-dir", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--run", default=None, help="label, e.g. with_skill / without_skill")
    args = ap.parse_args(argv)
    entry = load_eval(args.evals, args.id)
    with open(args.answer, encoding="utf-8") as fh:
        answer = fh.read()
    result = grade(entry, answer, args.outputs, args.skill_dir, args.run)
    with open(args.out, "w", encoding="utf-8") as fh:
        json.dump(result, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    print("%s: %d/%d" % (args.run or args.out, result["passed_count"], result["total"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
