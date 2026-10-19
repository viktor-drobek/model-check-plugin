#!/usr/bin/env python3
"""Admit a clean release-tag checkout and emit GitHub Actions outputs."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

TAG_PATTERN = re.compile(r"v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?")


def git(root, *args):
    try:
        return subprocess.check_output(
            ["git", "-C", str(root), *args], text=True,
            stderr=subprocess.PIPE, timeout=30,
        ).strip()
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
        raise ValueError("git release validation failed") from exc


def validate(root, tag):
    if not TAG_PATTERN.fullmatch(tag):
        raise ValueError("invalid release tag: expected [v]MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]")
    commit = git(root, "rev-parse", "--verify", f"refs/tags/{tag}^{{commit}}")
    if git(root, "rev-parse", "HEAD") != commit:
        raise ValueError("checkout does not match release tag")
    versions = []
    for name in ("plugin.json", ".claude-plugin/plugin.json"):
        with (root / name).open(encoding="utf-8") as source:
            versions.append(json.load(source)["version"])
    if versions[0] != versions[1]:
        raise ValueError("manifest versions differ")
    version = tag.removeprefix("v")
    if versions[0] != version:
        raise ValueError("release tag does not match plugin version")
    if git(root, "status", "--porcelain", "--untracked-files=no"):
        raise ValueError("release checkout is dirty")
    return {"tag": tag, "version": version, "source_commit": commit}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    args = parser.parse_args()
    try:
        outputs = validate(Path(__file__).resolve().parent.parent, args.tag)
    except (ValueError, OSError, KeyError, TypeError) as exc:
        print(f"validate-release: {exc}", file=sys.stderr)
        return 1
    for key, value in outputs.items():
        print(f"{key}={value}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
