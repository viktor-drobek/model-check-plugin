# Counterexamples: prefix + loop, decoding, and telling the four causes apart (FR-015)

Sources: `model-check-skill-notes/05-principles-of-model-checking.md` гл. 4
(lasso = stem + loop; project product states back to the system; accepting
cycle, not just accepting state), гл. 6 (CTL: path or tree witness);
`model-check-skill-notes/09-verification-of-automata-programs.md` гл. 4 (reverse
mapping to source names and phases; two different correct lassos for one
property), гл. 2; `model-check-skill-notes/10-cross-book-synthesis.md` §10
(diagnostic pipeline, first causal fork, classification), §3.6 (classified
outcomes); `model-check-skill-notes/11-skill-requirements.md` §1.3 (three
independent defect sources: model, formula, mapping), §11 (`violated`
obligations), §12 (spurious traces), FR-015, FR-023, NFR-011, AC-18;
`model-check-skill-notes/07-lectures-01-09.md` лекция 3 (over-approximation makes
counterexamples possibly infeasible); `model-check-skill-notes/02-design-and-validation-of-computer-protocols.md`
гл. 13–14 (trail as the central artefact; the first transition after which the
violation is inevitable).

A counterexample is the main interface between the engine and the engineer
(10 §10). `mc_explain` produces it; this file tells you how to read it, present it,
and — the part the engine cannot do — decide what it means.

## 1. Shapes

| Property kind | Counterexample shape | Notes |
|---|---|---|
| `invariant`, `assert`, `reach` (as a witness) | finite path from an initial state to the bad (or sought) state | BFS gives the shortest by number of steps; DFS gives the first found |
| `deadlock` | finite path to a state with no enabled transition; the list of processes and where each is blocked | `end`-labelled processes are shown as legitimately finished |
| `ltl` | **prefix + loop** (lasso): a finite stem, then a cycle that repeats forever; the acceptance obligation that is never discharged is named (which `<>`/`U` promise stays open) | for a liveness formula the loop is essential — a finite sequence alone refutes nothing (AC-18); for a safety formula there is no `loop` at all: the prefix is already a bad prefix (05 гл. 3). A never claim that reaches its own end is also a violation on a finite prefix, with no loop (`properties-ltl-ctl.md` §6) |
| `progress` | prefix + loop where the loop visits no `progress` label; the claim process is `np_` | a model with **no** `progress` label at all makes every cycle non-progress — read such a `violated` as "no progress labels were placed", not as a finding (`properties-ltl-ctl.md` §7) |
| `ctl` — violation of `AG p`, witness for `EF p` | one finite path | |
| `ctl` — witness for `EG p` | prefix + loop inside `p`-states | |
| `ctl` — violation of nested formulas such as `AG EF p`, `AG(p -> AF q)` | may be a **tree**: a state from which *every* continuation fails; `mc_explain` returns the path to that state and, per branch, why it fails | say in the report whether one trace was enough (11 §11) |
| `invalid-model` | finite path to the step that overflowed a domain or a capacity | this is a model defect, not a property result |

## 2. Reading `mc_explain` output

Each step records: the process that moved, the statement (with file and line from
the mapping), the variables whose values changed (old → new), sends and receives
(channel, message), and for LTL the property-automaton state — which you drop when
presenting (projection, 10 §10). The loop is marked with its entry state; the
prefix and the loop are shown separately (FR-015).

Present a counterexample as:

1. **One sentence**: what is violated and in how many steps ("both users reach
   `L7` after 11 steps; `cnt` becomes 2").
2. **The chronology** in the user's names (NFR-011): step, who, what, what changed.
   Never in engine-internal names; if the mapping lacks a name, say so rather than
   inventing one.
3. **The first causal fork** (10 §10; 02 гл. 14): the earliest step after which the
   violation became possible or inevitable — usually a scheduling choice between two
   enabled processes, or a nondeterministic branch. Mark it. The shortest trace is
   not always the most explanatory one; the fork is what the engineer needs.
4. **For a lasso**: the loop as a separate block (`prefix` then `loop`, the split of
   §2a), with the obligation that is never met ("`ack` is promised by `<>` and never
   occurs in the loop"), and — if fairness was `none` — which enabled process or
   transition is starved along the loop (this feeds `fairness.md` §3). Say plainly
   that the loop repeats forever; a reader who takes it for a finite trace will
   count the steps and ask what happens next.
5. **Parameters** the trace depends on (10 §10): queue sizes, process counts, loss
   switches, fairness mode, search mode, seed, model hash — from `mc_manifest`.

The corpus `*.trail` files (`CH14/version3.trail`, `CH14/version6.trail`,
`CH15/client_server.pml.trail`) show why decoding matters: a SPIN trail is a list
of `depth:process:transition` numbers, unreadable without the model, the mapping
and the tool version (11 §15). `mc_explain` exists so that the report never ships
numbers like these without their meaning.

## 2a. The lasso as the engine emits it (G4)

A counterexample for a temporal property carries `loop` {`start`, `steps`}:
**`loop.start` is the 1-based index of the first step of the cycle**, and
`loop.steps` is how many steps the cycle has. So steps `1 … loop.start - 1` are the
prefix and steps `loop.start … loop.start + loop.steps - 1` repeat forever; after
the last loop step the state is exactly the state before `loop.start`. A finite
counterexample has no `loop` field. `mc_explain` does the split for you, into
`prefix` and `loop`, and states it in one sentence in `loop_note`; with the CLI you
do the arithmetic yourself from the report and write that sentence by hand. The
report's `summary` marks the cycle with `; loop: `.

Three process names in a lasso are not processes of the user's system, and all
three must be dropped or explained rather than narrated as actions:

| `process` | What it is | What to say |
|---|---|---|
| `never:<property id>` (`never:ltl1` for the first `--ltl`; `never:never` when the property is the model's own `never` claim) | the property automaton for the **negated** formula, moving first at every step | drop it from the chronology (projection, 10 §10); if you show it, say it is the property, not the system |
| `np_` | the synthesised non-progress automaton of a `--progress` search | the same; its `accept` state means "the cycle so far visited no progress label" |
| `-` | a **null step**: no process moved | see below — there are two reasons for one, and they mean different things |

**Reading a stuttering process in a lasso.** A step of process `-` appears in two
situations, and confusing them inverts the diagnosis:

1. **Stutter extension — and it exists for `ltl` only.** The command text says so:
   `(stutter: no process can move, the system state repeats forever; SPIN's stutter
   extension)`. The system has *stopped* — every process terminated, or the state is
   a deadlock — and the engine extends the finite run into an infinite one by
   repeating the final state, because an LTL formula is evaluated on infinite runs.
   A loop made only of the claim and such a step means: **nothing further happens,
   and the promise is never kept**. Report it as "the system reaches a state from
   which nothing can happen, and `q` never occurs", and check the `deadlock`
   property next to it — for an `ltl` property, a stutter loop on a non-terminated
   state is a deadlock wearing a liveness costume.

   **Which searches extend and which do not** (the rule, not an exception to one):

   | Property kind | A state where no process can move | Why |
   |---|---|---|
   | `ltl`, and a model's own `never` claim | extended: the final state repeats forever, and the trace shows it as a step of process `-` | an LTL formula is evaluated on infinite runs, and `pan -a` does the same |
   | `progress` (the `np_` search) | **not** extended: the state has no successors at all, so no cycle passes through it and no such counterexample exists | `pan -l` does the same. A blocked system is a *deadlock*, and reporting it as a non-progress cycle would hide one defect behind another — they have different fixes (10 §10). The deadlock still reaches you, from the safety search, as `deadlock` `violated` |
   | `ctl` | the state carries a **self-loop**, so the transition relation is total and `EG`/`AF` are defined there; the property record's `temporal.note` says so | the same purpose as the extension, by the mechanism CTL needs |

   So a `progress` counterexample never contains a stutter step, and a blocked model
   answers `--progress` with `progress` `verified` beside `deadlock` `violated`. If
   you meet a `progress` `violated` whose loop looks like "nothing happens", the loop
   is a real cycle of the model, not a stopped system — read it again.
   (Engine: `explore/cycle.go`, `noStutter`, set exactly when the property kind is
   `progress`; `steps/g5-addendum-confirmation.md` §1–§3.)
2. **Weak-fairness bookkeeping.** Under `fairness: weak` the copies construction
   inserts null steps to advance the fairness copy. They carry no system meaning at
   all; drop them silently.

A process that is *absent* from the loop is the third case and the interesting one
for fairness: it did not stutter, it was **not scheduled**. Under `fairness: none`
the engine names it for you — `reason` says which processes move in the loop and
which are "enabled throughout the loop and never move". That sentence is the input
to question 3 of §3 and to `fairness.md` §3.

## 3. The four causes — classification procedure

A `violated` result is a fact about the model and the formula. What it means about
the system is a judgement you make and justify (11 §1.3: model, formula and
mapping are three independent sources of defect; 10 §3.6, §10). Run the questions
in order; the first "yes" gives the class. A trace can carry more than one defect:
after the first is fixed, rerun and classify the new trace afresh.

| # | Question | If yes → class | Typical evidence |
|---|---|---|---|
| 1 | Is the formula saying something other than the requirement? (wrong polarity, wrong atom, `[]<>` where `<>[]` was meant, LTL where the requirement was branching, missing sanity conjunct, vacuity) | **property defect** | `mc_lint_property` polarity/vacuity notes; paraphrase the formula and compare with the user's words; corpus: `CH4/prop.pml` shows `[]p` vs `![]p` as never claims |
| 2 | Does a step in the trace do something the real system cannot do? (only askable when a real system is named — see the paragraph after the table) (an interleaving hidden by a real lock but not by the model, a channel losing when the real one cannot, a capacity or domain smaller than reality, a missing `end` label, an environment allowed too much) | **model defect** (including translation/mapping defects and over-approximation artefacts) | compare each step with the system; 07 лекция 3: an over-approximating model admits traces the program does not have |
| 3 | Does the loop rely on a scheduling that the user's stated assumptions exclude? (a ready process never runs; a message is lost forever) | **fairness / environment artefact** — logically a sub-case of 2, singled out because the remedy differs (a justified assumption, not a model change); handled by `fairness.md` §3: rerun with the assumption and report both | the starved transition in the loop |
| 4 | None of the above: every step is possible in the real system — or there is no real system, and every step is possible in the object the user described — and the formula says what the requirement says | **system defect** (read the paragraph below before writing the words) | the trace replayed by `mc_simulate` in `guided` mode from the counterexample id |

When the model **is** the object — a Petri net, or an algorithm the user gave in words or as a `.pml` file, with no real system behind it (E3 and E1 of the evals) — questions 2 and 4 cannot be asked as written: there is no real system to compare a step against. Ask them of the object the user described instead, and write the class as "defect of the described object", not "system defect": the bare phrase reads as a statement about an implementation, and the intake card's "relation to the implementation" field is empty by construction. Watch for the contradiction this produces if you do not: a report that says in its summary "nothing follows about the real system" and in its classification "every step is possible in the real system" has used the word *system* in two senses and says both nothing and something about the same thing.

"Spurious" in the sense of abstraction refinement (11 §12, FR-018) is class 2: the
engine performs no abstraction itself, so a spurious trace can only come from the
user's or your abstraction of the system. The check is feasibility: walk the trace
against the real system's rules. If it is infeasible, do not report a system defect;
report the model defect, refine the model (add the missing constraint), rerun all
properties.

For every class the report says what to do next, and it never edits the requirement
to make the trace disappear (11 §11, last bullet). A property defect is fixed by
rewriting the formula *with the user*; a model defect by changing the model and
recording the change; a fairness artefact by a justified assumption reported as a
separate result; a system defect by proposing a fix as a hypothesis and rechecking
all properties after the user applies it.

## 4. Two different lassos can both be right

09 гл. 4 records two tools giving two different counterexamples for the same
liveness property of an ATM model (the user cancels forever; the machine switches
off into a terminal state) and calls both correct. Expect the same here: DFS and
BFS, or two budgets, may return different traces. That is not a contradiction. If
the user needs a specific scenario, run `mc_simulate` in guided mode, or add a
`reach` property that pins the scenario.

## 5. Minimisation (FR-023)

Prefer BFS when the user wants the shortest safety counterexample. For lassos,
shorter is not automatically clearer. Any minimisation you do by hand (dropping
steps) must keep the trace replayable by `mc_simulate`; a step you drop because it
"looks irrelevant" may change what is enabled later (10 §10, causal slicing). When
in doubt, show the full trace and highlight the causal slice.

## 6. Turning a counterexample into a regression check

Record the scenario as a `reach` property (the bad state is reachable) or keep the
guided step list. After the fix, rerun: the `reach` property should become
`verified` on `[] !bad` (unreachable), and every other property must be rerun too.
