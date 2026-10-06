# Performance plan, step 4 — the two ends of a buffered channel

Layer: G0 (`explore`, a small addition to `ir`). Follows `perf3-confirmation.md`.
Protocol: `BUILD-PROTOCOL.md` step 6.

## What was asked, and what was measured instead

The plan said: widen the partial-order reduction's coverage, starting with the
temporal properties, because only the shared counters and the vacuity watch
block a model with a never claim. Before writing a line I measured what that
would unlock. The ten corpus models refused for a temporal property have 1 to
217 states, and their temporal searches are as big as the safety search, which
the reduction would shrink; the two large real models (`client_server`, 191 000
states; `leader`, 41 000) are refused for process creation and, behind it, for
rendezvous channels, channels named by value, `_nr_pr`, `empty`/`nempty` and an
`atomic` run. Lifting any single refusal would have unlocked nothing that needed
it. What a textbook protocol model is made of is processes passing messages along
buffered channels (`CH5/sink_source_filter.pml`: source, filter and sink), and
there the reduction applied and shrank nothing, because any two processes on one
channel were a conflict. So this step widens coverage where it matters, and the
temporal-property increment is dropped, with the figures above as the reason.

## The rule

On a buffered channel a send and a receive are independent. When both are
enabled the buffer is neither full nor empty; the send appends at the tail, the
receive removes the head and shifts the rest down, zeroing the freed slot, so the
buffer is the same bytes in either order; neither disables the other (a send
leaves the head alone, which is all a `Match` looks at; a receive only makes
room). When they are not both enabled there is nothing to commute. So the send
end and the receive end of a channel are separate cells. Two sends, or two
receives, share an end and conflict; every other use of a channel (its length, a
clear, a channel named by a value) is a use of the whole channel, which overlaps
both ends.

What the independence does not cover: the other end can ENABLE an alternative of
the process expanded alone (the receiver's pop frees a full channel; the sender's
push gives an empty one a message), and an alternative of the ample edges that
another process enables is exactly what C1 forbids. So the plan records the
channel operations among the edges of each location, and `pick` expands the
process alone only in a state where each send has room and each receive finds a
message. A receive blocked by a head that does not match cannot be enabled by a
sender, which writes at the tail. The ends stay whole for the edges of a
location with an else, for a d_step edge and for the locations a d_step enters.

`ir` gets a test that the walk over every expression slot of the IR, which the
process-table test relies on, visits all of them. (The first version exported
the walk for a check of channel lengths that turned out to be redundant; the
export went with it.)

## Two things the first version had wrong, found by testing the tests

1. **My own conditions were redundant.** The task I wrote required exactly one
   sender and one receiver, no clear, nobody reading the length. A mutant that
   dropped each condition survived the random oracle, and when I asked why, the
   answer was that the cells already do it: two senders conflict on the send end,
   a clear and a length read are whole-channel cells, and a rendezvous is refused
   before it matters. The conditions were code with no effect on any verdict and
   no test that could tell. They are gone, and the rule is the simpler and more
   general one above (it covers many producers and one consumer, the usual
   shape). The generalisation was then run, not trusted: 240 000 random models,
   no disagreement.
2. **The mutation harness reported a failing baseline as "killed".** A stale
   test (it asserted that a sender and a receiver of one channel conflict, which
   is no longer true) failed on the unmutated code, and the harness counted every
   mutant as killed by it: most of the first "all killed" was empty. The harness
   now runs the baseline first and refuses to go on if it is red. With a valid
   baseline four mutants survived: two were dead code (removed), and two were
   real holes in the tests, both fixed: a trap in which the process that goes
   first by index hid a missing requirement on the other end (both orders are
   now tested), and a d_step edge whose own channel operation was protected only
   by its continuation.

## The random oracle did not test this rule, until it did

With the generator as it was, none of the mutants of the new rule (dynamic
requirement dropped, send room not required, receive message not required, else
and d_step edges treated as ends) was killed by the oracle alone, over 8 000
models: the shared globals of the generic generator make every pair of processes
conflict anyway. `randomPipelineModel` builds the shape the rule is for: the
processes share nothing but a global that nobody writes, so a guard on it is a
branch no one can take (it leaves a state without a move behind it when the
branch is lost), channels have a chosen sender and receiver, locations mix sends,
receives that wait for a value at the head, free edges and such branches, and one
model in four gets a second sender or receiver, one in eight clears a channel,
some read a length, some use an else or a d_step. Now the oracle alone kills the
dynamic-requirement mutants, the else/d_step mutant and a mutant that checks the
requirement for the first operation only. It still does not kill mutants that
change precision, not soundness (ends not told apart; a d_step edge treated as
an end), nor, on that sample, "visibility ignored"; the directed tests kill those.

## Behaviour pinned

- `explore/por_channels_test.go`: seventeen analysis tests (producer and consumer,
  several alternatives, a pipeline, two senders, two receivers, a process at both
  ends, a length read in a guard, an assert, an effect, a match and a property,
  a channel named by value, a clear, an else, a d_step, the rendezvous refusal,
  the visible cells, a private channel).
- `explore/por_channels_search_test.go`: a pipeline shrinks to under half with
  the same verdicts; the two enabling traps, a full channel whose send only the
  pop enables and an empty one whose receive only the push enables, each in both
  process orders, keep their deadlock; the requirement is about the state (with
  room the same shape is reduced); two senders keep the order that matters; a
  length read keeps what it observes.
- `features/g7-por.feature`: four scenarios on `por-pipeline.pml`,
  `por-fullchan.pml`, `por-emptychan.pml` and `por-order.pml`.
- Mutants: 23 of the rule and of its neighbours, each killed by at least one
  test; baseline checked green first.
- 240 000 random models before the third review and another 240 000 after it,
  on generators whose channels carry one to three fields (seeds from 1, 11 000 000
  and 13 000 000, 80 000 each; about 24% are reduced): same verdicts, same states without a move, stored
  states a subset, every counterexample replayed, error reachability.

## Third cross-review (HEAD `0bf7d89`): approve with changes, one real hole

`codex-terra-high`, `coddy-gemma` and `claude-fable` all said *approve with
changes*; `coddy-gemma` was cut off at its output limit once and answered on the
retry, citing line numbers that do not exist in a 738-line file, and retracted
its own findings. **No unsound verdict in the code**: the orchestrator ran about
14.7 million models of its own generators, whose messages have one to three
fields and whose receives match any field and bind into arrays, plus an
exhaustive enumeration of small scopes (three processes, two channels, up to two
of nine kinds of operation per location, 9.7 million models) and a Promela
fuzzer through the real frontend, with no disagreement. 25% of its models had
the dynamic requirement veto an expansion.

What it found, all of it about the tests, one of it about soundness:

- **A clear overlaps the receive end: pinned by nothing.** A mutant that wrote
  `ClearChans` as the send end only passed every test, the random oracle and the
  root package, and answered `verified` for an assert that is violated. Model:
  P sends on c and then clears it; Q receives from c and then asserts false. I
  reproduced it on a binary built from the mutant before touching anything.
  `TestPORAClearOverlapsTheReceiveEndAndKeepsTheAssert` (both process orders) and
  an analysis test now pin it.
- **The requirement of a second channel, and of the location it is read at, were
  killed only by the random oracle.** Mutants that always read channel 0, or the
  requirement of location 0, survived every directed test. My first attempt at a
  model for the second did not kill it, because the trap was reachable round the
  state that matters (the pop could come before P got to the alternatives), and
  my second was no better: P and Q both sent on d, so P was never eligible there
  and the dynamic condition was never asked. The model that works has P the only
  sender on d, a requirement at P's first location that holds in the later state
  while the one that matters does not, and flags that make every way to the
  deadlock pass through the state in which P is at the alternatives and d is full
  (`twoChannelTrap`, in four process orders, with and without the first stage).
- **The state the requirement reads.** A mutant that read `s.next`, the scratch
  successor of an earlier speculative firing, instead of `s.cur` survived
  everything, including about 40 000 of the orchestrator's models and its
  exhaustive scope. It could not build a model where that gives a wrong verdict,
  and I could not either: the ancestor on the stack would already have been
  expanded with that alternative. It stays an open question about reach, closed
  as a test: `TestPORChannelsCanActReadsTheStateBeingExpanded` poisons `s.next`
  and checks nine states, which also pins the capacity (a mutant that took it as 1
  survived) and the channel index.
- **An eligible process without enabled moves** must be passed over, not end the
  search for an ample set: a mutant that stopped at it reduced nothing, and only
  conservatism of that kind survived. `TestPORAnEligibleProcessWithoutMovesIsPassedOver`.
- **A receive blocked by a head that does not match**, enabled by another
  receiver's pop (the two receives share the receive end): a search-level model.
- The generators' channels now carry one to three fields, and their receives
  bind, match or ignore each field.
- Wording: "under a depth budget" is "under any budget" (states, time and memory
  make the same asymmetry); a comment on the requirement loop said an operation
  read whole "would not be eligible", true only when another process uses the
  channel.

Rejected after checking by the orchestrator: remote variable references (outside
the subset), poll and sorted-send operators (rejected by the parser), an `index`
with a wrong arity (`ir.Check`), a missing CLI end-to-end test (thirteen
scenarios of `g7-por.feature` and the G2 scenarios run the tool).

Not worth fixing: mutants that only lose reduction power (the room check against
the capacity, a receive that binds an array element read as a write of the whole
array, the requirement recorded for a d_step edge or an else location), which is
documented as deliberate.

## Results

| Model | Full | `--por` before this step | `--por` now |
|---|---:|---:|---:|
| `por-pipeline.pml` K=3 | 579 states | 138 | **40** |
| `por-pipeline.pml` K=6 | 1 827 | not measured | 73 |
| `por-pipeline.pml` K=10 | 3 555 | not measured | 117 |
| `por-fullchan.pml` (a send only the pop enables) | 10, deadlock | 9 | 8, deadlock kept |
| `por-emptychan.pml` (a receive only the push enables) | 8, deadlock | 8 | 6, deadlock kept |
| `por-order.pml` (two senders, order matters) | 21, assert violated | 21 | 19, assert violated |

The states of the pipeline now grow with K and not with the interleavings of
its stages (117 at K=10 against 3 555).

On the textbook corpus nothing else shrinks: 86 models compared, 16 shrink, and
the four more than before are the four fixtures of this step. `sink_source_filter`
still ends on the state budget with and without `--por`: its states come from the
contents of channels of eight slots over three values, a choice of data and not
an order of steps, which a reduction of interleavings does not touch. Saying so
is the honest reading of "applied and nothing reduced".

## Not done

- The refusals that matter for the big corpus models: rendezvous, process
  creation, `atomic`, channels named by value, `_nr_pr`. Each needs its own
  argument and a review of its own; none is unlocked by this step.
- A requirement for a channel that nobody else touches is recorded though it
  cannot be needed (it can only delay an expansion, never lose a verdict).
- The tracked binaries in `engine/bin/` are not rebuilt.
