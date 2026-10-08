# testdata/unbounded

Models on which a search without a depth budget never ends (an atomic block whose
loop never exits: the process keeps the exclusive control for ever). They are kept
out of `testdata/promela` because the corpus-wide tests (`por_corpus_test.go`,
`parallel_corpus_test.go`, and the oracles that walk that directory) run the
sequential depth-first search with a state budget only, which does not bound an
atomic sequence, and such a model grows the stack until the process is killed. The
command line and the MCP server always carry a depth budget (default 1 000 000) and
a time budget, and answer `inconclusive`.

`atomic-never-ends.pml` is the model of the scenarios "a breadth-first search over
an atomic sequence that never ends is bounded" and "the estimate of a model with an
atomic sequence that never ends comes back" of `features/g1-promela.feature`.
