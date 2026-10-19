# AsyncAPI declaration profile

`AnalyzeAsyncAPI` accepts only caller-supplied JSON for AsyncAPI `2.6.0` or
`3.0.0`. It is an offline declaration adapter. It does not contact a broker,
fetch or expand references, infer direction from names, execute messages, or
claim transport conformance or runtime behavior.

The document requires `info.title`, `info.version`, and `channels`. Optional
`info.description` is retained as an exact `asyncapi.unmapped_declaration` gap
at `#/info/description` in both versions. Channel
names are bounded non-empty names: they are at most 256 bytes, do not begin
with `/`, do not contain `//`, `..`, or `\`, and contain no empty slash-separated
component. Surrounding Unicode whitespace and embedded `http:` or `https:`
(case-insensitive) are also rejected in channel names. Invalid names and unknown
members at consumed structural scopes are rejected with `stage4.schema_invalid`.

## Version-specific accepted subset

### AsyncAPI 2.6.0

The root may contain only:

```text
asyncapi id info servers defaultContentType channels operations components tags externalDocs
```

`operations` is rejected in a 2.6.0 document. Each channel may contain only:

```text
description subscribe publish parameters bindings servers
```

A `publish` or `subscribe` operation may contain only:

```text
operationId message summary description tags externalDocs bindings traits security
```

It must contain one `message`. `operationId`, when supplied, must be a bounded
contract name and must be unique across all v2 channel operations. An identifier
beginning with `http:` or `https:` (case-insensitive) is rejected even when its
message is reference-only. Message objects may contain only:

```text
$ref name title summary description headers payload correlationId contentType tags externalDocs bindings examples traits
```

A reference-only message must contain only `$ref`; otherwise a message may have
an optional payload. A direct payload schema is checked against the shared
schema-key allowlist and its direct properties become `BinaryField` facts.

### AsyncAPI 3.0.0

The root has the same allowlist, but may additionally contain `operations`;
`operations` is processed only for version 3.0.0. Each channel may contain
only:

```text
address messages description parameters bindings servers tags externalDocs title summary
```

Channel `messages` must be an object of bounded contract names to message
objects. The v3 channel operation shape is represented by root `operations`.
Each operation may contain only:

```text
action channel title summary description tags externalDocs bindings traits security messages
```

`action` must be `send` or `receive`. `channel` must be an object containing
only `$ref`, and that reference must point to a channel in the same document
using `#/channels/<name>` (with JSON Pointer escaping). An operation is recorded
as `asyncapi.operation_unmapped`; its action is not transport-behavior
evidence. If present, operation `messages` must be an array of reference-only
objects. Every supplied operation metadata member (`title`, `summary`,
`description`, `tags`, `externalDocs`, `bindings`, `traits`, `security`, and
`messages`) also receives an exact `asyncapi.unmapped_declaration` gap. Each
reference at `messages[i]` receives its own gap at that element's exact pointer.

The v3 message object has the same exact allowlist as v2. A channel message may
be a `$ref`-only message or a message with an optional payload. Components may
contain only:

```text
messages schemas securitySchemes parameters correlationIds operationTraits messageTraits serverBindings channelBindings operationBindings messageBindings
```

Only `components.messages` and `components.schemas` are mapped. The other
component collections are admitted as `asyncapi.unmapped_declaration` gaps.
Component schemas and message payload schemas use the same schema allowlist as
the OpenAPI profile; only direct properties of a message payload or component
schema become facts. Nested property schemas are still checked against that
allowlist, while remaining gap-only. Reference-only `items`,
`additionalProperties`, and `not` children emit their reference gap at the
child schema location (for example, `/items`) without a duplicate parent gap.
Schema property names beginning with `http:` or `https:` (case-insensitive)
are rejected at every depth of traversed message-payload and component schemas,
including their nested gap-only schemas. Schema-shaped data inside opaque
members such as message `headers`, parameters, or traits is not traversed as a
schema; it follows the shared parent-scoped URL-gap rule described below.

## Facts, gaps, and references

Direct channels become `Endpoint` facts. Direct messages and component schemas
become `ProtocolMessage` facts. Direct properties of message payloads and
component schemas become `BinaryField` facts. Recognized metadata that is not
mapped is retained as an explicit `asyncapi.unmapped_declaration` gap. Local
references are validated and recorded as `asyncapi.reference_unexpanded` gaps;
references are never expanded. Every accepted document has exactly one root
`asyncapi.declarations_only` gap, and every emitted evidence item is at level
`contract`. No fact or gap upgrades a declaration into an execution result,
source linkage, conformance result, or model verdict.

## Shared fail-closed admission and bounds

The shared contract scan visits every decoded JSON key and string before the
AsyncAPI parser handles gap-only content. It rejects broad secret indicators
case-insensitively in keys, channel names, routes, enum-like values, and opaque
content: `authorization`, `cookie`, `password`, `passwd`, `secret`, `token`,
`apikey`, `api_key`, `api-key`, `privatekey`, `private_key`, `private-key`, and
`credential`. Thus a `password` field, `/oauth/token`-like name, or `apiKey`
security declaration may be rejected even when AsyncAPI would otherwise admit
that member. This is intentional fail-closed compatibility behavior; the guard
must not be weakened or bypassed for a metadata or gap-only section.

URL detection is scoped to strings or keys beginning with `http:` or `https:`
(case-insensitive). A URL-valued declaration is allowed only beneath an
explicitly recorded non-root gap; the root declarations-only gap is not an
exception. A map key requires a gap enclosing its container; the gap recorded
for the key's own value cannot exempt that key. External references remain
rejected, and no URL is fetched. This
check does not document rejection of embedded prose URLs, alternate schemes,
or protocol-relative forms. In a mixed operation gap, a URL is still rejected
when the same gap covers a fact-producing descendant; nested gap-only members
remain independently eligible. For example, an operation `message.name` URL
is rejected, while a sibling `operation.summary` URL is admitted through its
own `asyncapi.unmapped_declaration` gap.

The shared admission limits are: 1 MiB artifact input, JSON depth 16, at most
4096 decoded JSON values, 256 local references, and at most 1024 planned facts
and 1024 planned gaps. Rejection is all-or-nothing and returns no partial
ledger. See [OpenAPI profile](openapi-profile.md) for the shared schema-key
allowlist and its explicit exclusions.

Each AsyncAPI 2 `publish` or `subscribe` operation is retained as an
`asyncapi.operation_unmapped` gap at the operation pointer; it is not transport-
behavior evidence. The shared schema-key names do not imply equivalent schema
semantics: this profile accepts only a string `type` value (not a type array),
and a `$ref` payload must be reference-only rather than having sibling keys.
These are deliberate profile constraints. Local references are checked for
pointer validity and object existence; channel-local message roles are not
resolved or enforced.
