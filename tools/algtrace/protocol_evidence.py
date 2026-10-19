#!/usr/bin/env python3
"""Import packet and standards evidence into a language-neutral protocol graph.

The program never captures traffic, downloads standards, or treats a parser's
output as proof of compliance. It imports immutable local artefacts and labels
the source of every claim.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any

SCHEMA = "protocol-evidence/v1"
MAX_INPUT_BYTES = 16 * 1024 * 1024
BCP14_RE = re.compile(r"\b(MUST NOT|SHALL NOT|SHOULD NOT|NOT RECOMMENDED|MUST|REQUIRED|SHALL|SHOULD|RECOMMENDED|MAY|OPTIONAL)\b")
SECTION_RE = re.compile(r"^\s*(\d+(?:\.\d+)*)\.\s+(.+?)\s*$")
RFC_RE = re.compile(r"\bRFC\s+(\d+)\b", re.IGNORECASE)
RFC_HEADER_RE = re.compile(r"^\s*Request for Comments:\s*(\d+)\b", re.IGNORECASE | re.MULTILINE)
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


def fail(message: str) -> None:
    print(f"protocol-evidence: {message}", file=sys.stderr)
    raise SystemExit(2)


def read_bytes(path: Path) -> bytes:
    if not path.exists() or not path.is_file() or path.is_symlink():
        fail(f"input must be a regular non-symlink file: {path}")
    data = path.read_bytes()
    if len(data) > MAX_INPUT_BYTES:
        fail(f"input exceeds {MAX_INPUT_BYTES} byte limit: {path}")
    return data


def artifact(path: Path, data: bytes, kind: str) -> dict[str, str]:
    return {"kind": kind, "name": path.name, "sha256": hashlib.sha256(data).hexdigest()}


def as_strings(value: Any) -> list[str]:
    if isinstance(value, list):
        return [str(item) for item in value]
    if value is None:
        return []
    return [str(value)]


def layer_value(layers: dict[str, Any], *names: str) -> str | None:
    for layer in layers.values():
        if not isinstance(layer, dict):
            continue
        for name in names:
            values = as_strings(layer.get(name))
            if values:
                return values[0]
    return None


def import_tshark(path: Path) -> dict[str, Any]:
    data = read_bytes(path)
    try:
        packets = json.loads(data)
    except json.JSONDecodeError as err:
        fail(f"invalid TShark JSON: {err}")
    if not isinstance(packets, list):
        fail("TShark JSON must be an array of packet records")

    nodes: dict[str, dict[str, str]] = {}
    edges: list[dict[str, str]] = []
    for ordinal, packet in enumerate(packets, start=1):
        try:
            layers = packet["_source"]["layers"]
        except (KeyError, TypeError):
            fail(f"packet {ordinal} has no _source.layers object")
        if not isinstance(layers, dict):
            fail(f"packet {ordinal} layers is not an object")
        number = layer_value(layers, "frame.number") or str(ordinal)
        source_ip = layer_value(layers, "ip.src", "ipv6.src")
        destination_ip = layer_value(layers, "ip.dst", "ipv6.dst")
        source_port = layer_value(layers, "tcp.srcport", "udp.srcport")
        destination_port = layer_value(layers, "tcp.dstport", "udp.dstport")
        if source_ip is None or destination_ip is None:
            fail(f"packet {number} lacks decoded IP endpoints")
        source = f"endpoint:{source_ip}:{source_port or '?'}"
        destination = f"endpoint:{destination_ip}:{destination_port or '?'}"
        message = f"packet:{number}"
        nodes[source] = {"id": source, "kind": "endpoint"}
        nodes[destination] = {"id": destination, "kind": "endpoint"}
        nodes[message] = {"id": message, "kind": "message"}
        edges.extend((
            {"evidence": "decoded", "kind": "sends", "source": source, "target": message},
            {"evidence": "decoded", "kind": "receives", "source": message, "target": destination},
        ))
        for protocol in sorted(name for name in layers if name not in {"frame", "eth", "ip", "ipv6", "tcp", "udp"}):
            protocol_node = f"protocol:{protocol}"
            nodes[protocol_node] = {"id": protocol_node, "kind": "protocol"}
            edges.append({"evidence": "decoded", "kind": "decodes_as", "source": message, "target": protocol_node})

    return document("tshark-json", path, data, nodes, edges)


def import_rfc(path: Path) -> dict[str, Any]:
    data = read_bytes(path)
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError:
        fail(f"RFC text is not UTF-8: {path}")
    match = RFC_HEADER_RE.search(text[:4096]) or RFC_RE.search(text[:4096])
    if match is None:
        fail("RFC text does not identify an RFC number in its first 4096 bytes")
    rfc_id = f"RFC{match.group(1)}"
    bcp14_declared = "BCP 14" in text and "RFC 2119" in text
    section = "preamble"
    requirements: list[dict[str, str]] = []
    for line_no, line in enumerate(text.splitlines(), start=1):
        heading = SECTION_RE.match(line)
        if heading:
            section = heading.group(1)
        for keyword in BCP14_RE.findall(line):
            requirements.append({
                "anchor": f"{rfc_id}:{section}:L{line_no}",
                "evidence": "normative_candidate" if bcp14_declared else "lexical",
                "keyword": keyword,
                "section": section,
                "text": line.strip(),
            })
    return {
        "artifact": artifact(path, data, "rfc-text"),
        "document": {"bcp14_declared": bcp14_declared, "id": rfc_id},
        "requirements": requirements,
        "schema": SCHEMA,
    }


def import_ieee_manifest(path: Path) -> dict[str, Any]:
    data = read_bytes(path)
    try:
        manifest = json.loads(data)
    except json.JSONDecodeError as err:
        fail(f"invalid IEEE clause manifest JSON: {err}")
    if not isinstance(manifest, dict):
        fail("IEEE clause manifest must be an object")
    standard = manifest.get("standard")
    document_hash = manifest.get("document_sha256")
    clauses = manifest.get("clauses")
    if not isinstance(standard, str) or not standard.startswith("IEEE "):
        fail("IEEE clause manifest requires a standard beginning with 'IEEE '")
    if not isinstance(document_hash, str) or not SHA256_RE.fullmatch(document_hash):
        fail("IEEE clause manifest requires a lower-case document_sha256")
    if not isinstance(clauses, list) or not clauses:
        fail("IEEE clause manifest requires at least one clause")
    imported: list[dict[str, str]] = []
    for item in clauses:
        if not isinstance(item, dict) or not isinstance(item.get("id"), str) or not isinstance(item.get("summary"), str):
            fail("each IEEE clause requires string id and summary")
        imported.append({"evidence": "licensed_clause_reference", "id": item["id"], "summary": item["summary"]})
    return {
        "artifact": artifact(path, data, "ieee-clause-manifest"),
        "document": {"id": standard, "sha256": document_hash},
        "requirements": imported,
        "schema": SCHEMA,
    }


def document(kind: str, path: Path, data: bytes, nodes: dict[str, dict[str, str]], edges: list[dict[str, str]]) -> dict[str, Any]:
    return {
        "artifact": artifact(path, data, kind),
        "edges": sorted(edges, key=lambda edge: (edge["source"], edge["target"], edge["kind"])),
        "nodes": [nodes[key] for key in sorted(nodes)],
        "schema": SCHEMA,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="import binary-protocol and standards evidence")
    subparsers = parser.add_subparsers(dest="command", required=True)
    for name in ("tshark", "rfc", "ieee-manifest"):
        command = subparsers.add_parser(name)
        command.add_argument("input", type=Path)
    args = parser.parse_args(argv)
    if args.command == "tshark":
        result = import_tshark(args.input)
    elif args.command == "rfc":
        result = import_rfc(args.input)
    else:
        result = import_ieee_manifest(args.input)
    json.dump(result, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except BrokenPipeError:
        raise SystemExit(0)
