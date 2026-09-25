#!/usr/bin/env python3
"""Measure how well a skill description triggers (plan 14 §8.2, G6).

The set is trigger-eval.json: {id, query, should_trigger, lang, why}. A judge is
shown an available-skills roster in which `model-check` carries the description
under test, together with decoy skills, and is asked, for each query, whether it
would consult that skill. The labels are never shown to the judge, and the
queries are presented in one seeded order so that the two classes are not
grouped.

Three subcommands:

  prepare  writes one judge prompt per run to <workdir>/<variant>/run-<n>.md and
           the key (ids in presentation order) to <workdir>/<variant>/key.json
  score    reads <workdir>/<variant>/run-<n>.json verdicts, takes the per-query
           majority over runs, and reports accuracy on the train and the test
           split separately
  report   collects several scored variants into trigger-results.json and names
           the chosen one

The split is stratified by class and deterministic in the seed, so the same seed
always yields the same held-out set; every variant is scored on that same split.
Accuracy compared with a threshold is the *test* one — the train figure is
reported beside it and never in its place.
"""

import argparse
import json
import os
import random
import sys

DECOYS = [
    ("logika", "Классическая формальная логика: проверить корректность рассуждения, найти "
               "логические ошибки в тексте, построить силлогизм, оценить индуктивный вывод."),
    ("code-review", "Review the current diff, or a PR number/branch/path target, for "
                    "correctness bugs and reuse/simplification/efficiency cleanups."),
    ("dataviz", "Use whenever you are about to create ANY chart, graph, plot, dashboard or "
                "data visualization, in any output medium."),
    ("pdf", "Read or extract text and tables from PDFs, merge, split, rotate, watermark, "
            "fill forms, OCR scanned PDFs."),
    ("docx", "Create, read, edit or manipulate Word documents (.docx) and templates (.dotx)."),
    ("security-review", "Complete a security review of the pending changes on the current branch."),
    ("run", "Launch and drive this project's app to see a change working: run, start or "
            "screenshot the app, or confirm a change works in the real app."),
    ("simplify", "Review the changed code for reuse, simplification, efficiency and altitude "
                 "cleanups, then apply the fixes. Quality only — it does not hunt for bugs."),
    ("claude-api", "Reference for the Claude API / Anthropic SDK — model ids, pricing, params, "
                   "streaming, tool use, MCP, agents, caching, token counting."),
    ("xlsx", "Open, read, edit or create spreadsheets (.xlsx, .csv, .tsv); clean or restructure "
             "messy tabular data. The deliverable must be a spreadsheet file."),
]

JUDGE_INSTRUCTIONS = """\
You are the dispatcher of a coding agent. Before answering a user's request you decide
which of the available skills, if any, you would open and read first.

Below is the roster of available skills, then {n} user requests. For each request decide
one thing only: **would you consult the `model-check` skill before answering it?**

Answer for every request. Answer "yes" only if you would actually open that skill for
that request; answer "no" if you would answer without it or would reach for a different
skill. Do not explain, do not hedge, do not answer the requests themselves.

## Available skills

{roster}

## Requests

{requests}

## Output

Write a JSON file to `{out}` and nothing else. Its content must be exactly a JSON object
mapping each request number (as a string) to "yes" or "no", for example:

{{"1": "no", "2": "yes", "3": "no"}}

Include every request number from 1 to {n}. Then reply with just the word DONE.
"""


# ----------------------------------------------------------------- helpers

def load_queries(path):
    with open(path, encoding="utf-8") as fh:
        qs = json.load(fh)
    if not isinstance(qs, list) or not qs:
        raise ValueError("trigger-eval.json must be a non-empty JSON array")
    seen = set()
    for q in qs:
        for field in ("id", "query", "should_trigger"):
            if field not in q:
                raise ValueError(f"entry {q.get('id', '?')} lacks {field}")
        if not isinstance(q["should_trigger"], bool):
            raise ValueError(f"entry {q['id']}: should_trigger must be a boolean")
        if q["query"] in seen:
            raise ValueError(f"duplicate query: {q['query'][:40]}…")
        seen.add(q["query"])
    return qs


def split(queries, seed=20260926, train_frac=0.6):
    """Stratified deterministic train/test split.

    Stratified because an unstratified 60/40 over 20 items can put every
    must-trigger query on one side, and an accuracy on a single-class split
    measures nothing. Returns (train_ids, test_ids), each sorted.
    """
    train, test = [], []
    for cls in (True, False):
        ids = sorted(q["id"] for q in queries if q["should_trigger"] is cls)
        rnd = random.Random(f"{seed}:{cls}")
        rnd.shuffle(ids)
        cut = round(len(ids) * train_frac)
        train += ids[:cut]
        test += ids[cut:]
    return sorted(train), sorted(test)


def presentation_order(queries, seed=20260926):
    """One fixed order for every run and every variant, classes interleaved."""
    ids = [q["id"] for q in queries]
    rnd = random.Random(f"order:{seed}")
    rnd.shuffle(ids)
    return ids


def majority(votes_per_run):
    """votes_per_run: list of {id: 'yes'|'no'}. Returns {id: bool} by majority.

    A query missing from a run does not vote. With an even split the answer is
    False ("would not consult"), so an unstable description is never credited
    with a trigger it produced only half the time.
    """
    out = {}
    ids = sorted({i for run in votes_per_run for i in run})
    for i in ids:
        yes = sum(1 for run in votes_per_run if run.get(i) == "yes")
        no = sum(1 for run in votes_per_run if run.get(i) == "no")
        out[i] = yes > no
    return out


def score(decisions, queries, ids):
    """Accuracy and the confusion counts over the given ids only."""
    want = {q["id"]: q["should_trigger"] for q in queries}
    tp = fp = tn = fn = 0
    missing = []
    for i in ids:
        if i not in decisions:
            missing.append(i)
            continue
        got, exp = decisions[i], want[i]
        if exp and got:
            tp += 1
        elif exp and not got:
            fn += 1
        elif not exp and got:
            fp += 1
        else:
            tn += 1
    n = tp + fp + tn + fn
    return {
        "n": n, "correct": tp + tn,
        "accuracy": round((tp + tn) / n, 4) if n else 0.0,
        "true_positive": tp, "false_negative": fn,
        "false_positive": fp, "true_negative": tn,
        "missing": missing,
    }


def roster_text(description, skill_name="model-check"):
    rows = [(skill_name, description)] + list(DECOYS)
    rows.sort(key=lambda r: r[0])
    return "\n".join(f"- **{n}**: {d.strip()}" for n, d in rows)


def judge_prompt(queries, description, out_json, seed=20260926):
    order = presentation_order(queries, seed)
    by_id = {q["id"]: q for q in queries}
    requests = "\n".join(f"{k}. {by_id[i]['query']}" for k, i in enumerate(order, 1))
    return order, JUDGE_INSTRUCTIONS.format(
        n=len(order), roster=roster_text(description),
        requests=requests, out=out_json)


# ------------------------------------------------------------ subcommands

def cmd_prepare(args):
    queries = load_queries(args.evalset)
    description = open(args.description, encoding="utf-8").read().strip()
    d = os.path.join(args.workdir, args.variant)
    os.makedirs(d, exist_ok=True)
    order = None
    for n in range(1, args.runs + 1):
        out_json = os.path.abspath(os.path.join(d, f"run-{n}.json"))
        order, text = judge_prompt(queries, description, out_json, args.seed)
        with open(os.path.join(d, f"run-{n}.md"), "w", encoding="utf-8") as fh:
            fh.write(text)
    train, test = split(queries, args.seed, args.train_frac)
    with open(os.path.join(d, "key.json"), "w", encoding="utf-8") as fh:
        json.dump({"order": order, "train": train, "test": test,
                   "seed": args.seed, "runs": args.runs,
                   "description": description}, fh, indent=2, ensure_ascii=False)
    print(f"{d}: {args.runs} prompts, {len(order)} queries, "
          f"train {len(train)} / test {len(test)}")


def cmd_score(args):
    queries = load_queries(args.evalset)
    d = os.path.join(args.workdir, args.variant)
    key = json.load(open(os.path.join(d, "key.json"), encoding="utf-8"))
    order = key["order"]
    runs = []
    for n in range(1, key["runs"] + 1):
        path = os.path.join(d, f"run-{n}.json")
        if not os.path.exists(path):
            print(f"warning: {path} missing, that run does not vote", file=sys.stderr)
            continue
        raw = json.load(open(path, encoding="utf-8"))
        runs.append({order[int(k) - 1]: str(v).strip().lower()
                     for k, v in raw.items() if 1 <= int(k) <= len(order)})
    if not runs:
        raise SystemExit("no run verdicts found")
    decisions = majority(runs)
    result = {
        "variant": args.variant,
        "runs_counted": len(runs),
        "seed": key["seed"],
        "split": {"train": key["train"], "test": key["test"]},
        "train": score(decisions, queries, key["train"]),
        "test": score(decisions, queries, key["test"]),
        "all": score(decisions, queries, [q["id"] for q in queries]),
        "per_run_agreement": round(
            sum(1 for i in decisions
                if len({r.get(i) for r in runs if i in r}) == 1) / max(len(decisions), 1), 4),
        "decisions": {i: decisions[i] for i in sorted(decisions)},
    }
    print(json.dumps(result, indent=2, ensure_ascii=False))
    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            json.dump(result, fh, indent=2, ensure_ascii=False)


def choose(variants):
    """The variant with the best held-out accuracy; a tie keeps the earlier one.

    Ties are decided for the incumbent on purpose: two descriptions that the
    held-out split cannot tell apart give no reason to replace a shipped one,
    and "best on the training split" is not allowed to break the tie, because
    that is the number the exit criterion refuses to be quoted.
    """
    best = max(v["test_accuracy"] for v in variants)
    return next(v for v in variants if v["test_accuracy"] == best)


def cmd_report(args):
    """Collect the scored variants into trigger-results.json.

    `chosen` is the variant whose *held-out* accuracy is highest; ties keep the
    earliest variant named on the command line, which is the incumbent, so a
    tie never rewrites a shipped description for nothing.
    """
    queries = load_queries(args.evalset)
    variants = []
    for name in args.variant:
        d = os.path.join(args.workdir, name)
        key = json.load(open(os.path.join(d, "key.json"), encoding="utf-8"))
        runs = []
        for n in range(1, key["runs"] + 1):
            path = os.path.join(d, f"run-{n}.json")
            if os.path.exists(path):
                raw = json.load(open(path, encoding="utf-8"))
                runs.append({key["order"][int(k) - 1]: str(v).strip().lower()
                             for k, v in raw.items()
                             if 1 <= int(k) <= len(key["order"])})
        dec = majority(runs)
        tr, te = score(dec, queries, key["train"]), score(dec, queries, key["test"])
        variants.append({
            "variant": name,
            "role": args.roles.get(name, ""),
            "description": key["description"],
            "runs": len(runs),
            "train_accuracy": tr["accuracy"], "train": tr,
            "test_accuracy": te["accuracy"], "test": te,
            "all_accuracy": score(dec, queries, [q["id"] for q in queries])["accuracy"],
            "decisions": {i: dec[i] for i in sorted(dec)},
        })
    chosen = choose(variants)
    key0 = json.load(open(os.path.join(args.workdir, args.variant[0], "key.json"),
                          encoding="utf-8"))
    doc = {
        "method": args.method,
        "judge_runs_per_variant": key0["runs"],
        "seed": key0["seed"],
        "split": {"train": key0["train"], "test": key0["test"],
                  "train_size": len(key0["train"]), "test_size": len(key0["test"]),
                  "stratified_by": "should_trigger"},
        "threshold_test_accuracy": args.threshold,
        "variants": variants,
        "chosen": chosen["variant"],
        "chosen_description": chosen["description"],
        "meets_threshold": chosen["test_accuracy"] >= args.threshold,
    }
    out = args.out or os.path.join(os.path.dirname(args.evalset), "trigger-results.json")
    with open(out, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    for v in variants:
        print(f"{v['variant']:8s} train {v['train_accuracy']:.4f}  "
              f"test {v['test_accuracy']:.4f}  ({v['role']})")
    print(f"chosen: {chosen['variant']} "
          f"(held-out {chosen['test_accuracy']:.4f}, threshold {args.threshold})")


def main(argv=None):
    here = os.path.dirname(os.path.abspath(__file__))
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--evalset", default=os.path.join(here, "trigger-eval.json"))
    p.add_argument("--workdir", default=os.path.join(here, "trigger-runs"))
    p.add_argument("--seed", type=int, default=20260926)
    p.add_argument("--train-frac", type=float, default=0.6)
    sub = p.add_subparsers(dest="cmd", required=True)

    a = sub.add_parser("prepare")
    a.add_argument("--variant", required=True)
    a.add_argument("--description", required=True, help="file holding the description text")
    a.add_argument("--runs", type=int, default=3)
    a.set_defaults(func=cmd_prepare)

    c = sub.add_parser("report")
    c.add_argument("--variant", action="append", required=True,
                   help="repeat; the first is the incumbent and wins ties")
    c.add_argument("--threshold", type=float, default=0.85)
    c.add_argument("--method", default="subagent judges (the headless `claude -p` CLI "
                                       "had no credentials in this environment)")
    c.add_argument("--role", action="append", default=[],
                   help="variant=role, for the report only")
    c.add_argument("--out")
    c.set_defaults(func=cmd_report)

    b = sub.add_parser("score")
    b.add_argument("--variant", required=True)
    b.add_argument("--out")
    b.set_defaults(func=cmd_score)

    args = p.parse_args(argv)
    if getattr(args, "role", None) is not None:
        args.roles = dict(r.split("=", 1) for r in args.role if "=" in r)
    return args.func(args)


if __name__ == "__main__":
    main()
