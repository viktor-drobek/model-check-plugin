# Issue #18 trace ledger — SARIF reference confirmation

This note records the final correction to the Stage 1 Trace Ledger SARIF
location rendering. It does not extend the Issue #18 implementation plan,
create a source adapter, or turn a model verdict into an implementation claim.

## Change confirmed

SARIF source regions are now emitted only for an unambiguous relative
`path:line` reference:

- the path is relative and contains a directory separator;
- it has no URI authority, query, or fragment marker; and
- the final suffix is a positive decimal line number.

All other references remain SARIF URIs without an invented source region. This
includes opaque URIs, network-path references, POSIX-absolute paths, and
references containing query or fragment syntax. Literal percent characters in
a source path are escaped before writing the SARIF URI, so consumers cannot
silently reinterpret a literal path character as an encoded byte.

The intentional ambiguity boundary is conservative: a bare `name:line` is an
opaque URI unless the ledger author writes an explicitly relative path such as
`./name:line`. Windows paths retain their existing path rendering, but do not
receive a source region.

## Evidence added

The correction is covered by:

- unit cases in `engine/trace/trace_test.go` for opaque URIs, URI-like dotted
  schemes, query and fragment suffixes, network paths, POSIX-absolute paths,
  Windows paths, and literal percent characters;
- G9 scenarios in `features/g9-trace-ledger.feature`; and
- trace fixtures for opaque URI, fragment, and POSIX-absolute gap references.

The user-facing boundary is documented in `docs/trace-ledger.md`.

## Blind review record

The final candidate was reviewed against the implementation plan and diff in
five blind rounds. The first four rounds required corrections to URI/path
classification and percent handling. Each finding was reproduced against the
candidate before the implementation or coverage changed.

The fifth round reviewed the resulting final candidate independently:

| Reviewer | Verdict |
| --- | --- |
| `claude-opus` | approve |
| `coddy-ndwal-gemma-4-31b` | approve |

The final quorum is therefore **2/2 approved**. The review covers the code and
its focused tests; this record is an audit note and is not a substitute for a
new source, model, or release review.

## Local validation

The following focused checks passed from `engine/`:

```text
go test ./trace
go test . -run '^TestG9TraceFeatures$'
```

After the final review correction, the complete local gate also passed from the
isolated candidate checkout:

```text
skills/model-check/assets/wait-for-capacity.sh --need-cores 2 --need-mem-mb 2048 --dir /tmp --min-disk-mb 256
cd engine && go test ./... && go vet ./... && test -z "$(gofmt -l .)"
```

The complete suite finished successfully: all packages passed; the root package
completed in about 380 seconds. The temporary `/tmp` monorepo links used only
to let existing root-sensitive feature checks find their rule files are not
part of the candidate or this repository change.

## Bounded local model confirmation

At the user's direction, the candidate was also checked on this host with a
fresh `mcd` binary built from commit `5e1661f`. This is a regression check of
the finite engine fixture below, not evidence about a Trace Ledger user,
protocol, or implementation.

| Field | Value |
| --- | --- |
| Model | `engine/testdata/promela/ctl-switch.pml` (`sha256:0788375397e79ab749086dcaf84e136ac698446bdbb537151fe8133d2759f160`) |
| System boundary | One untimed Promela process and one bounded byte variable; no external environment or messages |
| Property | CTL `AG EF (x == 0)` — every reachable state has a path to `x == 0` |
| Fairness | `none`; CTL fairness is outside the engine's supported semantics |
| Limits | 1,000 states, depth 1,000, 5,000 ms, 512 MiB, one worker |
| Capacity preflight | Passed twice: CPU 0%, memory 27–28%, 32 CPUs available |
| Estimate | Complete: 7 states, 8 transitions, depth 5 |
| Result | `ctl1`: `verified` with `exhaustive` evidence on the complete 7-state graph |

The built-in deadlock property also returned `verified` with `exhaustive`
evidence. The check completed well inside every declared limit. A universal
CTL property that holds has no single witness trace; its justification is the
complete reachable graph. This result therefore confirms only that property of
this finite fixture under the stated modelling assumptions.

## Outstanding confirmation gate

The required bounded remote model confirmation has **not** passed yet. The
the remote comparison host preflight reported available capacity, but the final candidate still needs
to be available on that host. The designated second remote-host attempt could
not authenticate with the configured SSH identity. No remote model result is
claimed here.

Before this work can be reported as fully confirmed, synchronize the committed
candidate through the repository’s normal remote workflow, restore authorized
access to the designated remote host, and perform the bounded confirmation on
both hosts while monitoring load.
