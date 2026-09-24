# IR testdata

`counters-10-5.json` and `counters-10-6.json` are the IR encoding of the
Spike's synthetic throughput model (`engine/internal/spike/counters.go`,
Promela in `internal/spike/testdata/counters.pml`): N processes, each with a
local byte counter incremented modulo K; the reachable graph is exactly K^N
states (100 000 and 1 000 000), every state has N enabled transitions, and
there is no deadlock. They are generated, not hand-written:

    MCD_GEN_TESTDATA=1 go test ./explore -run TestGenerateTestdata

The 1e5 model is used by `features/g0-engine.feature` for the budget
scenarios; both are used by the throughput measurement in
`steps/g0-confirmation.md` (K1 condition: interpreted IR within 20x of the
Spike's compiled-in models).
