# Pact v4 declaration profile

> **Status:** implementation profile under pre-review. It is not a Pact verifier,
> a provider test runner, or evidence that a consumer and provider conform.

`protocol.AnalyzePact` accepts caller-supplied JSON bytes through:

```go
protocol.ContractInput{
    Artifact: protocol.Input{Path: "contracts/example.pact.json", Content: data},
    ContentSHA256: sha256hex,
}
```

The adapter never opens the logical path, uses a network, starts a process,
loads Pact tooling, or executes matching rules, generators, payloads, or a
provider verification. It returns either a complete canonical
`mcd-trace-ledger/1` document or `nil, error`.

## Accepted input

The input is strict JSON, one UTF-8 value without a BOM or trailing data. The
shared contract boundary rejects duplicate decoded keys, nesting beyond 16,
unsafe host paths and traversal, external or unresolved references, reference
cycles, and secret indicators in every decoded key and string. The supplied
lowercase SHA-256 must match the supplied bytes.

Only this Pact v4 envelope is accepted:

```json
{
  "consumer": {"name": "consumer"},
  "provider": {"name": "provider"},
  "metadata": {"pactSpecification": {"version": "4.0"}},
  "interactions": []
}
```

`interactions` contains 1 through 256 entries. Every interaction requires a
unique `key` of 1 through 200 ASCII characters: the first character is a
letter, and subsequent characters may be letters, digits, `_`, `.`, `:`, or
`-`. `type` and `description` are separate required bounded strings; they are
not required to be unique.

### HTTP interactions

`type: "Synchronous/HTTP"` requires a direct `request` and `response`:

- request: uppercase standard `method` and a bounded absolute route `path`;
- response: integer `status` from 100 through 599.

The profile accepts request `query`, `headers`, `body`, `matchingRules`, and
`generators`, and response `headers`, `body`, `matchingRules`, and
`generators`, only as opaque declarations. Their presence is disclosed as
`pact.unmapped_declaration`; they are not evaluated or copied into labels.

Each accepted HTTP interaction emits one `Endpoint` plus direct request and
response `ProtocolMessage` nodes. The only emitted relations are direct
`derived_from` containment links from each message to the endpoint.

### Message interactions

`Asynchronous/Messages` and `Synchronous/Messages` are recognised only when
they contain at least one direct object or array in `contents`, `request`, or
`response`. They emit no endpoint, message, observation, direction, or
transport fact. Each accepted value is disclosed as
`pact.unmapped_declaration`.

Optional `pending`, `providerStates`, `comments`, and `transport` fields are
also gap-only declarations. Other fields and interaction types reject.

## Output and limits

Every node and containment edge has `contract` evidence, the full input hash,
a logical source anchor, and producer `mcd-pact-adapter` version `1`. Every
accepted document includes `pact.declarations_only`, explicitly stating that
this profile establishes no execution, verification, source linkage,
transport correctness, conformance, or model result.

The shared boundary limits input to 1 MiB, JSON values to 4096, references to
256, and planned declarations to 1024. Admission is process-wide and
nonblocking. Validation, planning, budgeting, ledger validation, and canonical
serialization are all-or-nothing. The 256-interaction limit is nominal: a
fully decorated input may hit the independent facts, gaps, values, or output
preflight caps sooner.

URL detection is scoped to strings or keys beginning with `http:` or `https:`
(case-insensitive). A URL-valued Pact declaration is allowed only beneath an
explicitly recorded non-root gap; the root `pact.declarations_only` gap is not
an exception. This includes gap-only HTTP/message members such as `query`,
`headers`, `body`, `matchingRules`, `generators`, and `comments`. A URL in a
fact-producing or otherwise consumed field remains rejected, and external
references are never fetched.

The facts and gaps limits are independent maxima (1024 each). Emitted edges and
evidence records add to the serialized ledger and are covered by the shared
output/working-memory preflight. Thus the nominal 256-interaction input limit
does not mean every decorated interaction consumes only one output record; the
fully decorated 256-interaction input is not guaranteed to be accepted; the
independent caps may reject it sooner.

HTTP `request.path` uses a separate bounded route grammar: a syntactically valid
route may resemble a filesystem path (for example `/etc/passwd`) and is not
rejected merely for that resemblance. Other Pact string values remain subject to
the shared host-path and secret-indicator scan.

Errors use `stage4.*` `RuleError` codes with constant safe locators. In
particular, unsupported version, malformed or unknown structure, duplicate
interaction key, unsafe/secret metadata, remote or cyclic references, and
budget exhaustion produce no output ledger.
