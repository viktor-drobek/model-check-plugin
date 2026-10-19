# Offline protocol artifacts

`engine/trace/protocol` accepts caller-supplied bytes and returns canonical trace-ledger JSON. It never opens the logical path, fetches a reference, reads a PCAP, invokes TShark or a grammar compiler, or constructs a protocol state machine. These are Go APIs, not new `mcd` commands or MCP tools.

## Supported-adapter matrix

| API | Accepted input | Output | Evidence and mandatory disclosures |
| --- | --- | --- | --- |
| `AnalyzeTShark(TSharkJSONInput)` | `tshark-json-projection/1`, with exact `TSharkVersion: "4.4.10"` and `Profile: "tshark-4.4.10-core-1"` | `mcd-trace-ledger/1` | `ProtocolMessage` and `BinaryField` presence facts with `decoded` evidence; producer and capture identity remain unverified. |
| `AnalyzeGrammar(GrammarFileInput)` | Exact supplied UTF-8 Kaitai `0.10` (`.ksy`) or Spicy `1.12.0` (`.spicy`) bytes, admitted by the conservative raw-byte profile | `mcd-trace-ledger/2` | One isolated `GrammarArtifact`, one `grammar` evidence record and one `grammar.anchor_unverified` gap. No syntax or compiler-success claim. |
| `AnalyzeGrammarReference(GrammarReferenceInput)` | Strict `mcd-grammar-reference/1` JSON manifest at a `.grammar-ref.json` logical path | `mcd-trace-ledger/1` | Only a `grammar.reference_unresolved` gap; no graph or evidence records. The target digest is an assertion, not verified grammar provenance. |

Unknown versions and profiles reject. The APIs need no optional external tools, so there is no tool-execution or fabricated-tool-result path. Capture acquisition, arbitrary `tshark -T json` output, raw payloads, dynamic fields, grammar syntax parsing, imports, compilation, and automatic merging of adapter outputs are outside this increment. The explicit offline runtime-span record is described separately in [Explicit offline runtime-span records](runtime-spans.md); the candidate Stage 4 contract profiles are described separately in [OpenAPI profile](openapi-profile.md), [Protobuf and Buf profiles](protobuf-profile.md), and [Pact profile](pact-profile.md). Those candidate implementations remain release-unverified and do not establish implementation conformance.

## Admission, limits and failures

The input `Input{Path, Content}` owns a caller-provided byte slice. Do not mutate or reuse that slice until the API returns. Each API checks the length, nonblockingly acquires a single process-wide construction slot, copies the bytes once, checks metadata syntax and secret indicators, verifies the caller's SHA-256 against the snapshot, and only then interprets the snapshot. Admission remains held through canonical serialization. The APIs return bytes, not a live document requiring serialization after admission.

Each artifact is **1 byte through 1 MiB**, inclusive. Its nonempty logical path is at most 512 ASCII characters in `[A-Za-z0-9._/-]`, relative and unpadded; empty, `.` or `..` segments, repeated/trailing slashes, absolute paths, drive/UNC paths, backslashes, URI schemes, queries and fragments reject. Grammar and reference paths require a nonempty basename stem before their required extension. A path is only a review anchor; the adapter does not open it.

JSON admits exactly one root value, no BOM, valid UTF-8 scalar values and paired surrogate escapes only. Duplicate names compare decoded, case-sensitive strings (`layers` and `\u006cayers` are the same name). The maximum container depth is 16, with the root array/object at depth 1. No tree is built before lexical, trailing-data, duplicate and depth checks complete. Incomplete or malformed roots select syntax rejection first; a completed root with any non-whitespace trailing byte selects trailing-data rejection before duplicate/depth checks. Without trailing data, duplicate/depth rejection uses the earliest offending byte.

Construction has independent fixed budgets of 420,003 records, 512 MiB canonical output, and a 768 MiB deterministic working-set estimate. Preflight uses concrete record strings and framing before graph-record construction. The working estimate is twice the output estimate plus 256 bytes per record plus 2 MiB parser overhead. Failure returns no partial ledger; it never truncates a successful subset.

Errors are `*protocol.RuleError` with `Code` and a non-secret `Locator`. They never include rejected input names, values or source fragments. In order, the stable codes are:

1. `stage3b.input_length`
2. `stage3b.resource_admission_busy`
3. `stage3b.metadata_invalid`
4. `stage3b.secret_indicator` (metadata)
5. `stage3b.digest_mismatch`
6. `stage3b.utf8_invalid` (structured input)
7. `stage3b.json_syntax`
8. `stage3b.json_trailing_data`, `stage3b.json_duplicate_name` or `stage3b.json_depth`
9. `stage3b.schema_invalid`
10. `stage3b.secret_indicator` (decoded structured names/values)
11. `stage3b.grammar_source_invalid` (raw grammar bytes)
12. `stage3b.preflight_limit`

Do not retry a rejected artifact with weaker checks. `resource_admission_busy` permits a later retry of the same immutable input after the other construction finishes; no artifact bytes were inspected by the busy call.

### Secret admission policy

Structured member names and all decoded string values, plus output-bound caller metadata, reject the unanchored case-insensitive RE2 expression `authorization|cookie|password|passwd|secret|token|api[_-]?key|private[_-]?key|credential`. Values and metadata also reject substring matches for `(?i)\b(?:bearer|basic)[ \t]+[A-Za-z0-9._~+/=-]{8,}`, `-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`, or `\bAKIA[0-9A-Z]{16}\b`. This narrow, high-false-positive policy is not a guarantee that arbitrary data contains no secrets.

Raw grammar bytes use the marker-word, PEM and AWS expressions over the entire file, including comments and quoted text. Standalone Bearer/Basic prose is not scanned by that raw policy. Every reverse solidus (`\`) rejects, preventing escape-based bypasses. Raw grammar admission also rejects invalid UTF-8, BOM, CR/CRLF, C0 controls other than LF/TAB, DEL, C1 controls, Unicode format characters and Unicode line/paragraph separators. The earliest raw-source violation determines the byte locator and always returns `grammar_source_invalid`, including marker hits.

## TShark projection

This synthetic projection is the complete envelope form, **not** arbitrary TShark JSON:

```json
[
  {
    "layers": [
      {"name": "frame", "fields": {"frame.protocols": "eth:ip:tcp"}},
      {"name": "eth", "fields": {"eth.type": "0x0800"}},
      {"name": "ip", "fields": {"ip.proto": "6"}},
      {"name": "tcp", "fields": {"tcp.srcport": "443", "tcp.dstport": "49152"}}
    ]
  }
]
```

The root contains 1–2,000 packet objects, each with exactly `layers`. Each `layers` array has 2–16 objects with exactly `name` and `fields`. The sole `frame` is first and has only `frame.protocols`, exactly the colon-joined sequence of following layer names, including repetitions. `frame` creates no ledger records. Repeated layers retain their distinct physical ordinals.

| Layer | Permitted fields | String-value domain |
| --- | --- | --- |
| `eth` | `eth.type` | `0x` plus four lowercase hexadecimal digits |
| `ip` | `ip.proto` | Canonical decimal 0–255 |
| `ipv6` | `ipv6.nxt` | Canonical decimal 0–255 |
| `tcp` | `tcp.srcport`, `tcp.dstport` | Canonical decimal 0–65535 |
| `udp` | `udp.srcport`, `udp.dstport` | Canonical decimal 0–65535 |
| `dns` | `dns.flags.response` | `0` or `1` |
| `dns` | `dns.qry.type`, `dns.resp.type` | Canonical decimal 0–65535 |

Canonical decimal has no leading zeroes except `0`. Only strings are accepted as field values. The recognised unsupported set is exactly `arp`, `http`, `http2`, `icmp`, `icmpv6`, `quic`, `tls`; each requires empty `fields` and yields an unsupported-dissector gap. Unknown layers/fields and fields in the wrong layer reject, not soft-gap.

Every supported post-frame layer emits a message node, including an empty layer. Every present permitted field emits a field node and a `derived_from` edge from field to layer; every node and edge gets one `decoded` evidence record. Values such as ports or protocol numbers are validated but **never stored**. IDs hash NUL-separated domain-specific tuples of the logical input identity, content digest, packet and layer-occurrence ordinals, layer name and (for a field) field name; identical exports at different logical paths remain distinct.

`CaptureID` is an ASCII `[A-Za-z0-9._-]{1,128}` assertion. `ExportStatus` is exactly `complete`, `truncated`, or `unknown`. All accepted exports retain `tshark.producer_unverified` and `tshark.capture_identity_unverified`; status controls only `tshark.export_truncated` / `tshark.export_status_unknown`. Missing permitted fields produce `tshark.permitted_fields_omitted`. Neither `complete` nor `decoded` proves exhaustive coverage, correct capture identity, decryption, endpoint acceptance, protocol reachability or implementation parsing correctness.

## Grammar registration and references

`GrammarFileInput` has `Artifact`, `Language`, `LanguageVersion`, `DeclaredAnchor`, and `GrammarSHA256`. `DeclaredAnchor` matches `[A-Za-z0-9_.:-]{1,200}`; it is not resolved against grammar syntax. Kaitai raw admission rejects every maximal ASCII word run `[A-Za-z0-9_]+` equal to `imports`; Spicy rejects `import` and `include`. These case-sensitive rules include quoted/commented/flow-style occurrences, but not `important` or `includes`. Passing raw admission does not prove that dependencies are absent (including opaque types).

Grammar anchors are `grammar:<language>@<version>:<logical-path>#declared=<anchor>`. IDs include the exact content digest, language, version, path and declared anchor. The node, evidence and mandatory gap have equal provenance from `mcd-grammar-adapter` version `1`.

A reference manifest has exactly these six string members:

```json
{
  "schema": "mcd-grammar-reference/1",
  "language": "kaitai",
  "language_version": "0.10",
  "target_id": "vendor.packet",
  "target_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "declared_anchor": "root"
}
```

`target_id` matches `[A-Za-z][A-Za-z0-9._:-]*` with at most 200 characters. `target_sha256` is exactly 64 lowercase hexadecimal characters, asserted for **absent** target bytes. Manifest provenance instead uses the verified hash of the consumed manifest. Its sole gap exposes both identities without conflating them. Structured escapes are allowed in this JSON and undergo decoded secret checks; the raw-grammar backslash ban does not apply to manifests.

## Go usage and verification

Build and test with the toolchain pinned in `engine/go.mod`; no additional dependency is required:

```sh
cd model-check-plugin/engine
go test ./trace/protocol -count=1
go test . -run '^TestG9SourceAdapterFeatures$' -count=1
```

A caller computes its assertion before calling the adapter; the adapter independently recomputes and verifies it:

```go
sum := sha256.Sum256(grammarBytes)
canonical, err := protocol.AnalyzeGrammar(protocol.GrammarFileInput{
    Artifact: protocol.Input{Path: "grammars/packet.ksy", Content: grammarBytes},
    Language: "kaitai", LanguageVersion: "0.10", DeclaredAnchor: "root",
    GrammarSHA256: fmt.Sprintf("%x", sum),
})
if err != nil {
    // Reject atomically; canonical is nil. Handle RuleError without logging input.
    return err
}
ledger, err := trace.UnmarshalJSON(canonical)
```

Imports are `crypto/sha256`, `fmt`, `modelcheck/trace` and `modelcheck/trace/protocol` inside this Go module. For TShark, supply `ExportSHA256` plus `CaptureID`, `TSharkVersion`, `Profile`, `ExportStatus`; for a reference manifest supply `ManifestSHA256`. Do not call `trace.JSON` again merely to serialize the returned bytes: admission already includes canonical serialization.

Fixtures in the unit and G9 tests are synthetic, generated inline and redistributable with this repository. They contain no third-party capture/grammar material. See [Trace ledger](trace-ledger.md), [the staged plan](issue-18-trace-ledger.md), and [provenance](../PROVENANCE.md).
