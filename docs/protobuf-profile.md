# Bounded Protobuf and Buf declaration profiles

The Stage 4 `protobuf` and `buf` adapters create **contract-level declaration
records only**. They do not execute a protocol, compile schemas, invoke
`protoc` or Buf, read imports or modules, fetch remote content, generate code,
or provide a conformance verdict.

## Protobuf profile

`AnalyzeProtobuf` accepts one caller-supplied UTF-8 source without a BOM. The
source must begin with:

```proto
syntax = "proto3";
```

One optional top-level `package` declaration is accepted. The remaining
accepted declarations are top-level `message` and `service` declarations:

- Identifiers use the protobuf grammar `[A-Za-z_][A-Za-z0-9_]*`. Package names
  may contain dot-separated identifiers, but empty package segments and dots
  in declaration, field, or RPC names are rejected.
- Field tags use decimal digits only. Multi-digit tags with a leading zero
  are rejected rather than interpreted as octal aliases; hexadecimal and
  negative tag forms are also outside this profile.
- Message fields may use scalar types, `repeated` scalar types, or a locally
  declared message type. `repeated` local message fields are conservatively
  rejected: this declaration profile does not explicitly model their
  cardinality contract.
- A service may declare unary `rpc Name (Input) returns (Output);` methods with
  locally declared message input and output types.
- A message creates a `ProtocolMessage` record; each field creates a contained
  `BinaryField`; each RPC creates an `Endpoint`.

Imports, options, enums, maps, oneofs, extensions, groups, nested messages,
streaming RPCs, unknown declarations, and malformed recovery syntax are
rejected. Map and field-option punctuation is recognized sufficiently to
return the stable `stage4.unsupported_declaration` rule rather than being
reported as an incidental lexical/schema error. The profile also rejects duplicate message/service names, duplicate
field names, colliding effective JSON field names (including collisions between an
explicit camelCase name and an inferred name). Inferred JSON names remove every
underscore, including consecutive and trailing underscores, so those aliases
also collide. Scalar vocabulary names are rejected for messages and services.
The profile also rejects duplicate RPC names, unresolved types, recursive local
message references, invalid tags, and tags in the reserved `19000..19999`
range.

Comments are lexical trivia and never produce declarations. The complete raw
source, including comments and the only admitted string literal, is scanned
before parsing for secrets and unsafe host paths. Labels and declaration
ordinals are bounded; source excerpts are never emitted.

## Buf profile

`AnalyzeBuf` accepts JSON only—the JSON-form YAML subset—not general YAML. The
entire accepted document is:

```json
{
  "version": "v2",
  "modules": [{"path": "api/v1"}],
  "deps": []
}
```

`modules` contains one through 32 unique canonical logical paths. `deps`, when
present, must be empty. A configured dependency or remote module is rejected,
not downloaded. Each local module yields a `buf.module_unresolved` gap; Buf
configuration creates no grammar, message, source, or execution provenance.

## Shared safety boundary

Both adapters receive caller-owned bytes and a SHA-256 digest. The shared
admission channel snapshots the bytes, validates the logical path and digest,
and releases admission after every success or failure. Output is canonicalized
as a Trace Ledger with `contract` evidence only. Every accepted input contains a
`*.declarations_only` gap that records the boundary: a declaration is neither
protocol execution, transport correctness, source linkage, nor a model-checking
result.

Inputs, lexical tokens, declarations, records, working memory, and output are
bounded. The lexical token cap is enforced while tokens are accumulated, and
token/declaration budget failures use locator `budget`. Rejections return
normalized `stage4.*` errors with no source text,
identifier, host-path, offset, tool diagnostic, or partial ledger output.

The stable `unsupported_declaration` classification also covers
`repeated map<...>` fields and RPC option blocks (`[...]` or `{...}`) after an
otherwise valid RPC signature. Other malformed punctuation remains
`schema_invalid`; this is a diagnostic boundary, not a promise to parse or
compile those declarations.

The raw Protobuf scanner rejects an UNC marker (`\\`) anywhere in the source;
the JSON contract scanner applies its host-path rule to decoded values and
rejects a value beginning with `\\`. This intentionally conservative difference
is part of the two input grammars, not a general filesystem-path compatibility
claim.
