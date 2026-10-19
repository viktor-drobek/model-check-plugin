# algtrace prototype

`algtrace.py` builds a deterministic JSON graph from a source tree. It is a
language-neutral first stage for reviewing algorithms and interaction protocols:
functions, calls, branches, loops, observable state writes, and obvious
send/receive operations become graph nodes and relations.

```bash
python3 tools/algtrace/algtrace.py path/to/source > algorithm-graph.json
python3 -m unittest tools/algtrace/test_algtrace.py
```

## Binary protocols and standards evidence

`protocol_evidence.py` imports three offline artefacts into a separate,
deterministic `protocol-evidence/v1` document:

```bash
# First create packet.json with a trusted local TShark installation:
tshark -r capture.pcapng -T json > packet.json
python3 tools/algtrace/protocol_evidence.py tshark packet.json > packet-graph.json

# RFC text is local input; uppercase BCP 14 words are candidates, not a proof.
python3 tools/algtrace/protocol_evidence.py rfc rfc8174.txt > rfc-requirements.json

# A licensed IEEE document stays outside the repository.
python3 tools/algtrace/protocol_evidence.py ieee-manifest ieee-clauses.json > ieee-requirements.json
```

TShark-derived message relations carry `decoded` evidence. They describe the
dissector's interpretation of the supplied capture, not an assertion that all
traffic was captured or decrypted. RFC requirements carry `normative_candidate`
only when the local document declares BCP 14/RFC 2119 terminology; a reviewer
must still select the actual requirement. IEEE input is deliberately a
hash-and-clause manifest with `licensed_clause_reference` evidence. It never
downloads, embeds or redistributes an IEEE PDF.

Use Kaitai Struct or Spicy to author/review binary grammars, and Netzob only for
explicitly labelled inferred message and state-machine hypotheses. Their output
must be added to the trace ledger as `grammar` or `inferred` evidence, never
silently promoted to a normative or verified claim.

## Supported source extensions

The CLI recognises C/C++, C#, Go, Java, JavaScript, Kotlin, Python, Ruby, Rust,
Swift and TypeScript source files. A directory may contain arbitrary other files;
they are ignored. Passing an unsupported file directly is rejected.

## Evidence boundary

Every relation has `"evidence": "syntactic"`. The tool does **not** resolve
types, imports, aliases, dynamic dispatch, generated code, macro expansion, or
runtime behaviour. In particular, `sends`, `receives`, and `writes_state` are
lexical indicators, not proof of a matching protocol peer or data race.

Use this output as a common trace layer. A later adapter may upgrade an edge with
CodeQL/CPG (`static`), Pact/Buf (`contract`), OpenTelemetry (`runtime`) or a model
checker (`verified`) evidence record; it must not overwrite the original evidence.

## Safety and determinism

The tool accepts regular UTF-8 files only, rejects symlinks, limits each source to
2 MiB, writes JSON solely to stdout and diagnostics solely to stderr. Node and edge
order is stable for the same inputs.
