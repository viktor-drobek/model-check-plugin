#!/usr/bin/env python3
"""Build a deterministic, language-neutral syntactic algorithm graph.

This prototype deliberately reports only facts visible in source syntax. It does
not claim type resolution, alias analysis, dynamic dispatch, or correctness of
an algorithm. Every emitted relation therefore carries evidence="syntactic".
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Iterable

SCHEMA = "algtrace/v1"
MAX_SOURCE_BYTES = 2 * 1024 * 1024

LANGUAGES = {
    ".c": "c",
    ".cc": "cpp",
    ".cpp": "cpp",
    ".cs": "csharp",
    ".cxx": "cpp",
    ".go": "go",
    ".h": "c",
    ".hpp": "cpp",
    ".java": "java",
    ".js": "javascript",
    ".jsx": "javascript",
    ".kt": "kotlin",
    ".kts": "kotlin",
    ".mjs": "javascript",
    ".py": "python",
    ".rb": "ruby",
    ".rs": "rust",
    ".swift": "swift",
    ".ts": "typescript",
    ".tsx": "typescript",
}

FUNCTION_PATTERNS: dict[str, tuple[re.Pattern[str], ...]] = {
    "go": (re.compile(r"^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\("),),
    "python": (re.compile(r"^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)\s*\("),),
    "rust": (re.compile(r"^\s*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w*)\s*\("),),
    "ruby": (re.compile(r"^\s*def\s+([A-Za-z_]\w*[!?=]?)"),),
    "javascript": (
        re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\("),
        re.compile(r"^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=.*=>"),
    ),
    "typescript": (
        re.compile(r"^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\s*\("),
        re.compile(r"^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=.*=>"),
    ),
    "java": (re.compile(r"^\s*(?:public|private|protected|static|final|synchronized|abstract|\s)+[\w<>\[\], ?]+\s+([A-Za-z_]\w*)\s*\("),),
    "kotlin": (re.compile(r"^\s*(?:public|private|protected|internal|suspend|inline|tailrec|\s)*fun\s+([A-Za-z_]\w*)\s*\("),),
    "c": (re.compile(r"^\s*[\w\s*]+\s+([A-Za-z_]\w*)\s*\([^;]*\)\s*\{"),),
    "cpp": (re.compile(r"^\s*[\w:<>,~\s*&*]+\s+([A-Za-z_]\w*)\s*\([^;]*\)\s*(?:const\s*)?\{"),),
    "csharp": (re.compile(r"^\s*(?:public|private|protected|internal|static|async|virtual|override|\s)+[\w<>\[\], ?]+\s+([A-Za-z_]\w*)\s*\("),),
    "swift": (re.compile(r"^\s*(?:public|private|internal|open|mutating|static|\s)*func\s+([A-Za-z_]\w*)\s*\("),),
}

BRANCH_RE = re.compile(r"^\s*(?:if|else\s+if|elif|switch|match|case)\b")
LOOP_RE = re.compile(r"^\s*(?:for|while|do)\b")
SEND_RE = re.compile(r"(?:<-\s*[^\s]|\.(?:send|publish|put|emit|enqueue|write)\s*\()")
RECEIVE_RE = re.compile(r"(?:<-\s*[A-Za-z_]|\.(?:recv|receive|get|take|read|dequeue)\s*\()")
STATE_WRITE_RE = re.compile(
    r"(?:\*\s*(?:state|shared|counter|balance)\s*=|\b(?:state|shared|counter|balance)\s*(?:\[[^]]+\])?\s*(?:=|\+=|-=))"
)
CALL_RE = re.compile(r"\b([A-Za-z_][\w$]*)\s*\(")
KEYWORDS = {"if", "for", "while", "switch", "match", "return", "func", "fn", "def", "new", "catch", "sizeof"}


def fail(message: str) -> None:
    print(f"algtrace: {message}", file=sys.stderr)
    raise SystemExit(2)


def language_for(path: Path) -> str | None:
    return LANGUAGES.get(path.suffix.lower())


def input_files(inputs: list[str]) -> list[Path]:
    files: list[Path] = []
    for raw in inputs:
        path = Path(raw)
        if not path.exists():
            fail(f"input does not exist: {path}")
        if path.is_symlink():
            fail(f"symlink input is not allowed: {path}")
        if path.is_file():
            if language_for(path) is None:
                fail(f"unsupported source extension: {path}")
            files.append(path)
            continue
        if not path.is_dir():
            fail(f"input is not a regular file or directory: {path}")
        for candidate in sorted(path.rglob("*")):
            if candidate.is_symlink():
                fail(f"symlink inside source tree is not allowed: {candidate}")
            if candidate.is_file() and language_for(candidate) is not None:
                files.append(candidate)
    unique = sorted(set(files), key=lambda item: str(item))
    if not unique:
        fail("no supported source files found")
    return unique


def display_path(path: Path, roots: Iterable[Path]) -> str:
    for root in roots:
        if root.is_dir():
            try:
                return str(path.relative_to(root))
            except ValueError:
                pass
    return path.name


def read_source(path: Path) -> list[str]:
    try:
        size = path.stat().st_size
    except OSError as err:
        fail(f"cannot stat {path}: {err}")
    if size > MAX_SOURCE_BYTES:
        fail(f"source exceeds {MAX_SOURCE_BYTES} byte limit: {path}")
    try:
        return path.read_text(encoding="utf-8").splitlines()
    except UnicodeDecodeError:
        fail(f"source is not valid UTF-8: {path}")
    except OSError as err:
        fail(f"cannot read {path}: {err}")
    raise AssertionError("unreachable")


def function_name(language: str, line: str) -> str | None:
    for pattern in FUNCTION_PATTERNS.get(language, ()):
        match = pattern.match(line)
        if match:
            return match.group(1)
    return None


def event_kinds(line: str) -> list[str]:
    kinds: list[str] = []
    if BRANCH_RE.search(line):
        kinds.append("branches")
    if LOOP_RE.search(line):
        kinds.append("loops")
    if SEND_RE.search(line):
        kinds.append("sends")
    if RECEIVE_RE.search(line):
        kinds.append("receives")
    if STATE_WRITE_RE.search(line):
        kinds.append("writes_state")
    return kinds


def add_node(nodes: dict[str, dict[str, object]], node: dict[str, object]) -> None:
    node_id = str(node["id"])
    nodes.setdefault(node_id, node)


def add_edge(edges: list[dict[str, object]], source: str, target: str, kind: str) -> None:
    edges.append({"evidence": "syntactic", "kind": kind, "source": source, "target": target})


def build_graph(inputs: list[str]) -> dict[str, object]:
    roots = [Path(item) for item in inputs]
    files = input_files(inputs)
    nodes: dict[str, dict[str, object]] = {}
    edges: list[dict[str, object]] = []
    function_ids: dict[tuple[str, int], str] = {}
    functions_by_name: dict[str, list[str]] = {}
    cached: list[tuple[Path, str, str, list[str]]] = []

    for path in files:
        language = language_for(path)
        assert language is not None
        rel = display_path(path, roots)
        lines = read_source(path)
        file_id = f"file:{rel}"
        add_node(nodes, {"id": file_id, "kind": "file", "language": language, "path": rel})
        cached.append((path, language, rel, lines))
        for line_no, line in enumerate(lines, start=1):
            name = function_name(language, line)
            if name is None:
                continue
            function_id = f"function:{rel}:{line_no}:{name}"
            function_ids[(rel, line_no)] = function_id
            functions_by_name.setdefault(name, []).append(function_id)
            add_node(nodes, {"id": function_id, "kind": "function", "language": language, "name": name, "path": rel, "line": line_no})
            add_edge(edges, file_id, function_id, "declares")

    for _, language, rel, lines in cached:
        file_id = f"file:{rel}"
        current = file_id
        for line_no, line in enumerate(lines, start=1):
            name = function_name(language, line)
            if name is not None:
                current = function_ids[(rel, line_no)]
            for kind in event_kinds(line):
                event_id = f"event:{rel}:{line_no}:{kind}"
                add_node(nodes, {"id": event_id, "kind": "event", "language": language, "path": rel, "line": line_no, "operation": kind})
                add_edge(edges, current, event_id, kind)
            for call in CALL_RE.findall(line):
                candidates = functions_by_name.get(call, [])
                if call in KEYWORDS or len(candidates) != 1 or current == candidates[0]:
                    continue
                add_edge(edges, current, candidates[0], "calls")

    edges.sort(key=lambda edge: (str(edge["source"]), str(edge["target"]), str(edge["kind"])))
    return {
        "edges": edges,
        "languages": sorted({language for _, language, _, _ in cached}),
        "nodes": [nodes[key] for key in sorted(nodes)],
        "schema": SCHEMA,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="build a language-neutral syntactic algorithm graph")
    parser.add_argument("paths", nargs="+", help="source files or directories")
    args = parser.parse_args(argv)
    json.dump(build_graph(args.paths), sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except BrokenPipeError:
        raise SystemExit(0)
