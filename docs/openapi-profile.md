# OpenAPI declaration profile

`AnalyzeOpenAPI` accepts only a caller-supplied JSON document with `openapi`
exactly `3.0.3` or `3.1.0`. It is an offline declaration adapter: it does not
open the logical artifact path, fetch URLs, resolve or expand references, execute
requests, generate code, or make a conformance or runtime claim.

The root requires `info.title`, `info.version`, and `paths`. Path operations are
limited to `get`, `put`, `post`, `delete`, `options`, `head`, `patch`, and
`trace`. Direct component schemas produce `ProtocolMessage` facts; direct schema
`properties` produce `BinaryField` facts with containment-only `derived_from`
links. Every accepted document has a root `openapi.declarations_only` gap and
all emitted evidence is at level `contract`.

## Exact schema-key allowlist

Every admitted schema object, including a direct property schema and nested
schema object, may contain only these 37 keys:

```text
$ref title description type format enum const default examples nullable
 deprecated readOnly writeOnly required properties items additionalProperties
 allOf anyOf oneOf not minimum maximum exclusiveMinimum exclusiveMaximum
 multipleOf minLength maxLength pattern minItems maxItems uniqueItems
 minProperties maxProperties discriminator xml externalDocs
```

The list is an admission allowlist, not a claim that every admitted keyword is
interpreted. Only direct `properties` are mapped to field facts. Nested property
schemas are still traversed against this allowlist, but remain
`openapi.unmapped_declaration` gaps rather than field facts. Other admitted
schema keywords are retained as `openapi.unmapped_declaration` gaps. References
are checked as local declarations but are not expanded into facts.

The following common OpenAPI or JSON Schema members are **not** admitted by this
profile and cause `stage4.schema_invalid`: `$id`, `$schema`, `$anchor`,
`$dynamicRef`, `$dynamicAnchor`, `$vocabulary`, `example`, `contentEncoding`,
`contentMediaType`, `contentSchema`, `patternProperties`,
`unevaluatedProperties`, `propertyNames`, `prefixItems`, `contains`,
`minContains`, `maxContains`, `if`, `then`, `else`, `dependentRequired`, and
`dependentSchemas`. Boolean schemas are also rejected. This is an exact narrow
profile; documentation must not be read as a promise of general OpenAPI or
JSON Schema support.

Unknown members are rejected at every consumed structural scope. The profile
also rejects malformed or unsafe routes, invalid response-code keys, duplicate
canonical path-template shapes (for example, `/a/{id}` and `/a/{name}`),
duplicate `operationId` values, missing responses, invalid names, unresolved or cyclic
local references, external references, and over-budget input. `info` is limited
to `title`, `version`, and `description`; root and operation metadata such as
servers, tags, security, callbacks, and external documentation may be admitted
only where implemented and are represented as explicit unmapped declaration
gaps. The profile does not accept YAML, webhooks, `2XX` response ranges, or
other unlisted OpenAPI members.

## Shared fail-closed admission

Before profile parsing, the shared contract admission scan visits every decoded
JSON key and string. Secret-indicator matching is deliberately broad and
fail-closed. It can reject a declaration because a marker occurs in a key,
route, enum-like value, or opaque/gap-only content; it does not attempt to
identify whether the value is an actual credential. The markers include
`authorization`, `cookie`, `password`, `passwd`, `secret`, `token`, `apikey`,
`api_key`, `api-key`, `privatekey`, `private_key`, `private-key`, and
`credential`, case-insensitively. Consequently, compatibility-sensitive
examples such as a `password` property, `/oauth/token`, or an `apiKey` security
scheme may be rejected before gap handling. Do not weaken or bypass this guard
by placing the declaration in a recognized metadata section.

URL handling is scoped, not a general URL-content scanner. A string or key is
classified by the current implementation when it begins with `http:` or
`https:` (case-insensitive). Such a URL-valued declaration is allowed only
beneath an explicitly recorded non-root gap; the root declarations-only gap
alone does not grant an exception. External `$ref` values remain rejected, and
no URL is fetched or executed. Embedded prose containing a URL, alternate
schemes, and protocol-relative forms are not thereby documented as rejected by
this check.

The shared bounds are part of the public contract: artifact input is capped at
1 MiB, JSON depth at 16, decoded JSON values at 4096, local references at 256,
and planned facts and gaps at 1024 each. Rejection is atomic: an error returns
no partial ledger.

The same profile allowlist is applied to both accepted document versions. In
particular, `const`, `examples`, `jsonSchemaDialect`, and
`components.pathItems` are admitted as profile declarations for either
`3.0.3` or `3.1.0`; this adapter does not claim native per-version OpenAPI
semantics for those members.
