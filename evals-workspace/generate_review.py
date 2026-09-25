#!/usr/bin/env python3
"""Generate the static review page for one evals-workspace iteration.

The skill-creator plugin, whose `eval-viewer/generate_review.py --static` produced
`iteration-2/review.html`, is not installed on this machine (checked with a
filesystem-wide search in the G3 stage-3 session). That page is nevertheless in the
repository, and it is a single self-contained file: one `const EMBEDDED_DATA = {...};`
line followed by the viewer application. This script reuses that page as the shell —
so the viewer is byte-for-byte skill-creator's, not a reimplementation — and rebuilds
only the data.

EMBEDDED_DATA, as the vendored viewer reads it:

  skill_name          str
  runs                [{id, prompt, eval_id, outputs: [{name, type, content}],
                        grading: <grader.py output>}]
                      Ordered with_skill first, then without_skill, each by eval id,
                      the same order aggregate.py uses.
  previous_outputs    {run_id: [{name, type, content}]}  — shown in a collapsed
                      "previous outputs" section of that run
  previous_feedback   {run_id: str}                      — one line above the run
  benchmark           the benchmark.json document        — the Benchmark tab

Usage:
  generate_review.py <iteration-dir> [--previous-workspace DIR] [--shell FILE]
                     [--skill-name NAME] [-o review.html] [--max-file-bytes N]
"""

import argparse
import json
import os
import re
import sys

CONFIGURATIONS = ("with_skill", "without_skill")
EMBEDDED_RE = re.compile(r"const EMBEDDED_DATA = (\{.*?\});\n", re.S)
TEXT_SUFFIXES = (".md", ".json", ".yaml", ".yml", ".txt", ".pml", ".pr", ".py",
                 ".sh", ".log", ".stderr", ".stdout", ".csv")


def load_json(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def is_text(name):
    return name.endswith(TEXT_SUFFIXES) or "." not in os.path.basename(name)


def collect_outputs(outputs_dir, max_bytes):
    """Every readable text artefact of one run, answer.md first, then sorted."""
    files = []
    for dirpath, dirnames, filenames in os.walk(outputs_dir):
        dirnames.sort()
        for name in sorted(filenames):
            path = os.path.join(dirpath, name)
            rel = os.path.relpath(path, outputs_dir)
            if not is_text(rel):
                continue
            try:
                with open(path, encoding="utf-8") as fh:
                    content = fh.read()
            except (UnicodeDecodeError, OSError):
                continue
            if len(content.encode("utf-8")) > max_bytes:
                content = content[:max_bytes] + (
                    "\n\n… truncated by generate_review.py at %d bytes; "
                    "the whole file is in the workspace at %s\n" % (max_bytes, rel))
            files.append({"name": rel, "type": "text", "content": content})
    files.sort(key=lambda f: (f["name"] != "answer.md", f["name"]))
    return files


def eval_dirs(iteration):
    """eval-* directories with a numeric id, sorted by it (aggregate.py's rule)."""
    out = []
    for name in sorted(os.listdir(iteration)):
        path = os.path.join(iteration, name)
        if not (name.startswith("eval-") and os.path.isdir(path)):
            continue
        meta_path = os.path.join(path, "eval_metadata.json")
        eval_id = None
        try:
            eval_id = int(name.split("-")[1])
        except (IndexError, ValueError):
            if os.path.exists(meta_path):
                eval_id = load_json(meta_path).get("eval_id")
        if not isinstance(eval_id, int):
            continue
        out.append((eval_id, path))
    return [(i, p) for i, p in sorted(out)]


def run_id(eval_dir, configuration):
    return "%s-%s" % (os.path.basename(eval_dir.rstrip(os.sep)), configuration)


def collect_runs(iteration, max_bytes):
    runs = []
    for configuration in CONFIGURATIONS:
        for eval_id, d in eval_dirs(iteration):
            grading_path = os.path.join(d, configuration, "grading.json")
            if not os.path.exists(grading_path):
                continue
            meta_path = os.path.join(d, "eval_metadata.json")
            meta = load_json(meta_path) if os.path.exists(meta_path) else {}
            runs.append({
                "id": run_id(d, configuration),
                "prompt": meta.get("prompt", ""),
                "eval_id": eval_id,
                "outputs": collect_outputs(os.path.join(d, configuration, "outputs"), max_bytes),
                "grading": load_json(grading_path),
            })
    return runs


def previous(previous_workspace, max_bytes):
    """previous_outputs and previous_feedback keyed by the SAME run id.

    Only evals that the previous iteration also ran appear; an eval introduced in
    this iteration simply has no previous section.
    """
    outputs, feedback = {}, {}
    if not previous_workspace:
        return outputs, feedback
    label = os.path.basename(previous_workspace.rstrip(os.sep))
    for _, d in eval_dirs(previous_workspace):
        for configuration in CONFIGURATIONS:
            grading_path = os.path.join(d, configuration, "grading.json")
            if not os.path.exists(grading_path):
                continue
            rid = run_id(d, configuration)
            outputs[rid] = collect_outputs(os.path.join(d, configuration, "outputs"), max_bytes)
            g = load_json(grading_path)
            failed = [e["text"] for e in g.get("expectations", []) if not e.get("passed")]
            line = "%s: %s/%s" % (label, g.get("passed_count"), g.get("total"))
            if failed:
                line += " — did not pass: " + "; ".join(failed)
            feedback[rid] = line
    return outputs, feedback


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("iteration")
    ap.add_argument("--previous-workspace", default=None,
                    help="an earlier iteration directory; its runs are shown beside this one's")
    ap.add_argument("--shell", default=None,
                    help="an existing review.html to take the viewer from "
                         "(default: <workspace>/iteration-2/review.html)")
    ap.add_argument("--skill-name", default="model-check")
    ap.add_argument("-o", "--out", default=None)
    ap.add_argument("--max-file-bytes", type=int, default=200000)
    args = ap.parse_args(argv)

    shell_path = args.shell or os.path.join(
        os.path.dirname(os.path.abspath(args.iteration.rstrip(os.sep))),
        "iteration-2", "review.html")
    with open(shell_path, encoding="utf-8") as fh:
        shell = fh.read()
    m = EMBEDDED_RE.search(shell)
    if not m:
        raise SystemExit("%s has no `const EMBEDDED_DATA = {...};` line" % shell_path)

    prev_outputs, prev_feedback = previous(args.previous_workspace, args.max_file_bytes)
    benchmark_path = os.path.join(args.iteration, "benchmark.json")
    data = {
        "skill_name": args.skill_name,
        "runs": collect_runs(args.iteration, args.max_file_bytes),
        "previous_feedback": prev_feedback,
        "previous_outputs": prev_outputs,
        "benchmark": load_json(benchmark_path) if os.path.exists(benchmark_path) else {},
    }
    if not data["runs"]:
        raise SystemExit("%s has no graded run" % args.iteration)

    page = (shell[:m.start()]
            + "const EMBEDDED_DATA = "
            + json.dumps(data, ensure_ascii=False, sort_keys=False)
            + ";\n"
            + shell[m.end():])
    out = args.out or os.path.join(args.iteration, "review.html")
    with open(out, "w", encoding="utf-8") as fh:
        fh.write(page)
    print("%d runs, %d with a previous run -> %s (%d bytes)"
          % (len(data["runs"]), sum(1 for r in data["runs"] if r["id"] in prev_outputs),
             out, len(page.encode("utf-8"))))
    return 0


if __name__ == "__main__":
    sys.exit(main())
