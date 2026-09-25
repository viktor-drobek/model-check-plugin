#!/usr/bin/env python3
"""Aggregate one evals-workspace iteration into benchmark.json and benchmark.md.

Layout read (one run per configuration, the layout grader.py and the
Cucumber steps use):

  <iteration>/
    eval-<id>-<name>/          (or the eval's own `dir`, e.g. eval-2b-…, whose
                                id is then read from eval_metadata.json)
      eval_metadata.json          {"eval_id", "eval_name", "prompt", ...}
      timing.json                 {"with_skill": {"tokens", "duration_s", "tool_uses"},
                                   "without_skill": {...}}
      with_skill/grading.json     grader.py output: {"expectations": [{"text",
      without_skill/grading.json   "passed", "evidence"}], "passed_count", "total"}

Output: the skill-creator benchmark.json schema (skill-creator
references/schemas.md — metadata, runs[] with result{pass_rate, passed, failed,
total, time_seconds, tokens, tool_calls, errors}, expectations, notes;
run_summary{with_skill, without_skill, delta}; notes[]) and the matching
benchmark.md. Configurations are always emitted with_skill first, then
without_skill (the viewer groups by the exact string "configuration").

The statistics (mean, sample stddev, min, max) and the markdown table follow
skill-creator scripts/aggregate_benchmark.py; that script expects run-N/
subdirectories and could not read this layout, so the logic is re-implemented
here on the standard library only. Deterministic: sorted eval ids, fixed
configuration order, the timestamp is passed in (or taken from
eval_metadata.json "date"), no wall-clock reads.

Usage:
  aggregate.py <iteration-dir> [--skill-name NAME] [--skill-path PATH]
               [--executor-model M] [--timestamp T] [-o benchmark.json]
"""

import argparse
import json
import math
import os
import sys

CONFIGURATIONS = ("with_skill", "without_skill")


def calculate_stats(values):
    """mean / sample stddev / min / max, rounded to 4 places (as skill-creator)."""
    if not values:
        return {"mean": 0.0, "stddev": 0.0, "min": 0.0, "max": 0.0}
    n = len(values)
    mean = sum(values) / n
    stddev = math.sqrt(sum((x - mean) ** 2 for x in values) / (n - 1)) if n > 1 else 0.0
    return {
        "mean": round(mean, 4),
        "stddev": round(stddev, 4),
        "min": round(min(values), 4),
        "max": round(max(values), 4),
    }


def load_json(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def dir_eval_id(path):
    """The eval id of one eval directory, or None when it has none.

    The directory is normally named eval-<id>-<name>, but an eval may carry a
    `dir` of its own (E2b is `eval-2b-starvation-loop`, a variant of eval 2).
    Then the id comes from eval_metadata.json, which every graded run has; a
    directory with neither is not an eval directory and is skipped.
    """
    name = os.path.basename(path.rstrip(os.sep))
    try:
        return int(name.split("-")[1])
    except (IndexError, ValueError):
        pass
    meta_path = os.path.join(path, "eval_metadata.json")
    if os.path.exists(meta_path):
        eval_id = load_json(meta_path).get("eval_id")
        if isinstance(eval_id, int):
            return eval_id
    return None


def eval_dirs(iteration):
    """eval-* directories sorted by eval id."""
    out = []
    for name in sorted(os.listdir(iteration)):
        path = os.path.join(iteration, name)
        if not (name.startswith("eval-") and os.path.isdir(path)):
            continue
        eval_id = dir_eval_id(path)
        if eval_id is None:
            continue
        out.append((eval_id, path))
    return [p for _, p in sorted(out)]


def load_run(eval_dir, configuration):
    """One run of one configuration, or None when it has no grading.json."""
    grading_path = os.path.join(eval_dir, configuration, "grading.json")
    if not os.path.exists(grading_path):
        return None
    grading = load_json(grading_path)
    meta_path = os.path.join(eval_dir, "eval_metadata.json")
    meta = load_json(meta_path) if os.path.exists(meta_path) else {}
    timing_path = os.path.join(eval_dir, "timing.json")
    timing = load_json(timing_path).get(configuration, {}) if os.path.exists(timing_path) else {}
    expectations = [
        {"text": e.get("text", ""), "passed": bool(e.get("passed")), "evidence": e.get("evidence", "")}
        for e in grading.get("expectations", [])
    ]
    total = len(expectations)
    passed = sum(1 for e in expectations if e["passed"])
    eval_id = meta.get("eval_id", grading.get("eval_id"))
    if eval_id is None:
        eval_id = dir_eval_id(eval_dir)
    return {
        "eval_id": eval_id,
        "eval_name": meta.get("eval_name") or grading.get("eval_name") or os.path.basename(eval_dir),
        "configuration": configuration,
        "run_number": 1,
        "result": {
            "pass_rate": round(passed / total, 4) if total else 0.0,
            "passed": passed,
            "failed": total - passed,
            "total": total,
            "time_seconds": float(timing.get("duration_s", 0.0)),
            "tokens": int(timing.get("tokens", 0)),
            "tool_calls": int(timing.get("tool_uses", 0)),
            "errors": 0,
        },
        "expectations": expectations,
        "notes": list(meta.get("notes", [])),
    }


def load_runs(iteration):
    """All runs, with_skill runs first (by eval id), then without_skill."""
    runs = []
    for configuration in CONFIGURATIONS:
        for eval_dir in eval_dirs(iteration):
            run = load_run(eval_dir, configuration)
            if run is not None:
                runs.append(run)
    return runs


def summarize(runs):
    """run_summary: per configuration stats in fixed order, then delta."""
    summary = {}
    for configuration in CONFIGURATIONS:
        mine = [r["result"] for r in runs if r["configuration"] == configuration]
        summary[configuration] = {
            "pass_rate": calculate_stats([r["pass_rate"] for r in mine]),
            "time_seconds": calculate_stats([r["time_seconds"] for r in mine]),
            "tokens": calculate_stats([r["tokens"] for r in mine]),
        }
    a, b = summary["with_skill"], summary["without_skill"]
    summary["delta"] = {
        "pass_rate": "%+.2f" % (a["pass_rate"]["mean"] - b["pass_rate"]["mean"]),
        "time_seconds": "%+.1f" % (a["time_seconds"]["mean"] - b["time_seconds"]["mean"]),
        "tokens": "%+.0f" % (a["tokens"]["mean"] - b["tokens"]["mean"]),
    }
    return summary


def observations(runs):
    """Mechanical notes: per-eval pass counts, assertions that pass in both
    configurations (do not differentiate the skill), assertions the skill
    run failed."""
    notes = []
    by_eval = {}
    for r in runs:
        by_eval.setdefault(r["eval_id"], {})[r["configuration"]] = r
    for eval_id in sorted(by_eval):
        pair = by_eval[eval_id]
        parts = []
        for configuration in CONFIGURATIONS:
            if configuration in pair:
                res = pair[configuration]["result"]
                parts.append("%s %d/%d" % (configuration, res["passed"], res["total"]))
        name = next(iter(pair.values()))["eval_name"]
        notes.append("Eval %s (%s): %s" % (eval_id, name, ", ".join(parts)))
        if len(pair) == 2:
            ws = {e["text"]: e["passed"] for e in pair["with_skill"]["expectations"]}
            wo = {e["text"]: e["passed"] for e in pair["without_skill"]["expectations"]}
            both = [t for t in ws if ws[t] and wo.get(t)]
            if both:
                notes.append("Eval %s: %d of %d assertions pass in both configurations and do not differentiate the skill: %s"
                             % (eval_id, len(both), len(ws), "; ".join("'%s'" % t for t in both)))
            failed = [t for t in ws if not ws[t]]
            if failed:
                notes.append("Eval %s: the with_skill run failed: %s" % (eval_id, "; ".join("'%s'" % t for t in failed)))
    return notes


def build_benchmark(iteration, skill_name, skill_path, executor_model, timestamp):
    runs = load_runs(iteration)
    if not runs:
        raise SystemExit("no graded runs under %s" % iteration)
    if not timestamp:
        dates = sorted({r for r in (
            (load_json(os.path.join(d, "eval_metadata.json")).get("date") if os.path.exists(os.path.join(d, "eval_metadata.json")) else None)
            for d in eval_dirs(iteration)) if r})
        timestamp = (dates[-1] + "T00:00:00Z") if dates else "unknown"
    return {
        "metadata": {
            "skill_name": skill_name,
            "skill_path": skill_path,
            "executor_model": executor_model,
            "analyzer_model": "none (mechanical aggregation, evals-workspace/aggregate.py)",
            "timestamp": timestamp,
            "evals_run": sorted({r["eval_id"] for r in runs}),
            "runs_per_configuration": 1,
        },
        "runs": runs,
        "run_summary": summarize(runs),
        "notes": observations(runs),
    }


def generate_markdown(benchmark):
    meta = benchmark["metadata"]
    summary = benchmark["run_summary"]
    a, b, d = summary["with_skill"], summary["without_skill"], summary["delta"]
    lines = [
        "# Skill Benchmark: %s" % meta["skill_name"],
        "",
        "**Model**: %s" % meta["executor_model"],
        "**Date**: %s" % meta["timestamp"],
        "**Evals**: %s (%d run(s) each per configuration)" % (
            ", ".join(str(e) for e in meta["evals_run"]), meta["runs_per_configuration"]),
        "",
        "## Summary",
        "",
        "| Metric | With Skill | Without Skill | Delta |",
        "|--------|------------|---------------|-------|",
        "| Pass Rate | %.0f%% ± %.0f%% | %.0f%% ± %.0f%% | %s |" % (
            a["pass_rate"]["mean"] * 100, a["pass_rate"]["stddev"] * 100,
            b["pass_rate"]["mean"] * 100, b["pass_rate"]["stddev"] * 100, d["pass_rate"]),
        "| Time | %.1fs ± %.1fs | %.1fs ± %.1fs | %ss |" % (
            a["time_seconds"]["mean"], a["time_seconds"]["stddev"],
            b["time_seconds"]["mean"], b["time_seconds"]["stddev"], d["time_seconds"]),
        "| Tokens | %.0f ± %.0f | %.0f ± %.0f | %s |" % (
            a["tokens"]["mean"], a["tokens"]["stddev"],
            b["tokens"]["mean"], b["tokens"]["stddev"], d["tokens"]),
        "",
        "## Per eval",
        "",
        "| Eval | Configuration | Passed | Pass rate | Time (s) | Tokens | Tool calls |",
        "|------|---------------|--------|-----------|----------|--------|------------|",
    ]
    for r in benchmark["runs"]:
        res = r["result"]
        lines.append("| %s %s | %s | %d/%d | %.0f%% | %.1f | %d | %d |" % (
            r["eval_id"], r["eval_name"], r["configuration"], res["passed"], res["total"],
            res["pass_rate"] * 100, res["time_seconds"], res["tokens"], res["tool_calls"]))
    if benchmark.get("notes"):
        lines += ["", "## Notes", ""] + ["- %s" % n for n in benchmark["notes"]]
    return "\n".join(lines) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("iteration")
    ap.add_argument("--skill-name", default="model-check")
    ap.add_argument("--skill-path", default="model-check-plugin/skills/model-check")
    ap.add_argument("--executor-model", default="<model-name>")
    ap.add_argument("--timestamp", default=None, help="ISO timestamp; default: the latest eval_metadata.json date")
    ap.add_argument("-o", "--output", default=None, help="benchmark.json path (default <iteration>/benchmark.json)")
    args = ap.parse_args(argv)
    benchmark = build_benchmark(args.iteration, args.skill_name, args.skill_path, args.executor_model, args.timestamp)
    out_json = args.output or os.path.join(args.iteration, "benchmark.json")
    out_md = os.path.splitext(out_json)[0] + ".md"
    with open(out_json, "w", encoding="utf-8") as fh:
        json.dump(benchmark, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    with open(out_md, "w", encoding="utf-8") as fh:
        fh.write(generate_markdown(benchmark))
    s = benchmark["run_summary"]
    print("with_skill %.1f%%  without_skill %.1f%%  delta %s  -> %s, %s" % (
        s["with_skill"]["pass_rate"]["mean"] * 100, s["without_skill"]["pass_rate"]["mean"] * 100,
        s["delta"]["pass_rate"], out_json, out_md))
    return 0


if __name__ == "__main__":
    sys.exit(main())
