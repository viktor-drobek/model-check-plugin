# Source adapters

Stage 2 of Issue #18 begins with a language-neutral adapter boundary and an
in-memory Go AST adapter. An adapter accepts bytes supplied by its caller; it
never discovers or opens files, resolves imports, invokes external tools, or
runs a model checker.

## Evidence boundary

`source.AnalyzeGo` emits only `syntactic` evidence. It records a Go function as
a `SourceSymbol`. It emits a `calls` relation only when the callee is a direct
identifier, names exactly one package-level function in the same input, is not
shadowed by any declaration in the enclosing function (including a type
parameter), and comes from a file without a dot import. A selector, external
name, alias, dot-imported file, ambiguous declaration, shadowed name, or a call
inside a function literal produces an explicit `source.unresolved_call` gap
instead.

This is intentionally conservative. The adapter makes no claim about aliasing,
dispatch, reachability, data flow, import resolution, behaviour, or correctness.
Consumers must not turn its facts into `modelled` or `verified` evidence.

## Go API

```go
var adapter source.Adapter = source.GoAdapter{}
document, err := adapter.Analyze(source.Input{
    Path:    "internal/transport/send.go", // logical review anchor
    Content: sourceBytes,                    // supplied by the caller
})
```

The resulting document conforms to `mcd-trace-ledger/1`. Every artifact is
pinned by SHA-256 of the provided bytes and producer version
`mcd-go-ast-adapter/1`; `Path` is retained only as a stable anchor. A parse or
input error returns no partial document. `AnalyzeGo` is a convenience wrapper
around `GoAdapter{}.Analyze`.
