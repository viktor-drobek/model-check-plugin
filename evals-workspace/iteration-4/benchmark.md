# Skill Benchmark: model-check

**Model**: <model-name>
**Date**: 2026-09-27
**Evals**: 1, 2, 3, 4, 5, 6, 7 (1 run(s) each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|------------|---------------|-------|
| Pass Rate | 97% ± 5% | 60% ± 20% | +0.37 |
| Time | 383.5s ± 103.0s | 259.9s ± 167.3s | +123.6s |
| Tokens | 135396 ± 17979 | 63826 ± 16395 | +71570 |

## Per eval

| Eval | Configuration | Passed | Pass rate | Time (s) | Tokens | Tool calls |
|------|---------------|--------|-----------|----------|--------|------------|
| 1 mutex-invariant-violated | with_skill | 8/8 | 100% | 457.7 | 148172 | 31 |
| 2 alternating-bit-liveness-fairness | with_skill | 8/9 | 89% | 519.5 | 145695 | 32 |
| 3 petri-net-hang | with_skill | 9/10 | 90% | 412.0 | 148167 | 34 |
| 4 telephone-busy-liveness | with_skill | 9/9 | 100% | 442.7 | 154087 | 35 |
| 5 c-code-out-of-subset | with_skill | 7/7 | 100% | 223.3 | 108838 | 19 |
| 6 ctl-ag-ef-idle | with_skill | 7/7 | 100% | 324.2 | 115883 | 25 |
| 7 starvation-loop-fairness | with_skill | 9/9 | 100% | 305.0 | 126929 | 23 |
| 1 mutex-invariant-violated | without_skill | 6/8 | 75% | 334.8 | 72814 | 27 |
| 2 alternating-bit-liveness-fairness | without_skill | 5/9 | 56% | 435.5 | 82100 | 26 |
| 3 petri-net-hang | without_skill | 5/10 | 50% | 103.0 | 47445 | 5 |
| 4 telephone-busy-liveness | without_skill | 3/9 | 33% | 178.8 | 56053 | 13 |
| 5 c-code-out-of-subset | without_skill | 3/7 | 43% | 143.9 | 51308 | 17 |
| 6 ctl-ag-ef-idle | without_skill | 5/7 | 71% | 513.6 | 86810 | 25 |
| 7 starvation-loop-fairness | without_skill | 8/9 | 89% | 109.8 | 50251 | 12 |

## Notes

- Eval 1 (mutex-invariant-violated): with_skill 8/8, without_skill 6/8
- Eval 1: 6 of 8 assertions pass in both configurations and do not differentiate the skill: 'The mutual-exclusion property is reported with status violated'; 'The evidence level exhaustive is stated'; 'The counterexample is decoded through the model's labels L1–L4'; 'The counterexample shows both users at label L7 (the critical section)'; 'The cause is classified as a system (algorithm) defect'; 'The engine version and input hash (manifest) are quoted'
- Eval 2 (alternating-bit-liveness-fairness): with_skill 8/9, without_skill 5/9
- Eval 2: 5 of 9 assertions pass in both configurations and do not differentiate the skill: 'Fairness is raised as a question or as an explicitly labelled assumption'; 'The formula that was actually checked is shown, in LTL syntax'; 'The answer says the verdict does not depend on the fairness setting (the model is lock-step)'; 'The claim is limited to this model: it has no message loss, retransmission or timeout'; 'No forbidden phrasing'
- Eval 2: the with_skill run failed: 'The engine was invoked (CLI or MCP), not reasoned around'
- Eval 3 (petri-net-hang): with_skill 9/10, without_skill 5/10
- Eval 3: 5 of 10 assertions pass in both configurations and do not differentiate the skill: 'The counterexample is the firing sequence t1 then t4'; 'The final marking has one token in p2 (p2 = 1, or set notation {p2, p5})'; 'The final marking has one token in p5 (p5 = 1, or set notation {p2, p5})'; 'The number of reachable markings (6) is quoted from the engine's counters'; 'No forbidden phrasing (no errors / proved / net is correct)'
- Eval 3: the with_skill run failed: 'The engine CLI was run on the net and the command line is shown (mcd check --petri, the binary possibly named through a shell variable)'
- Eval 4 (telephone-busy-liveness): with_skill 9/9, without_skill 3/9
- Eval 4: 3 of 9 assertions pass in both configurations and do not differentiate the skill: 'It is formalised as a progress label or an LTL eventually/infinitely-often formula'; 'The formula refers to the model's labels or states (Busy), not to printf output'; 'Fairness is raised before the result'
- Eval 5 (c-code-out-of-subset): with_skill 7/7, without_skill 3/7
- Eval 5: 3 of 7 assertions pass in both configurations and do not differentiate the skill: 'The rejected construct c_code is named'; 'No property result is imitated for the original model: no line of the answer attributes verified/violated/inconclusive to simple1.pr, c_code/c_expr or the original model/file (a verdict about a clearly separate rewrite is allowed — including a line that calls that rewrite "the original" one as against a mutant; refined in iteration-2 and again in iteration-3, see steps/g3-evals3-confirmation.md)'; 'No host code is executed and the answer says so'
- Eval 6 (ctl-ag-ef-idle): with_skill 7/7, without_skill 5/7
- Eval 6: 5 of 7 assertions pass in both configurations and do not differentiate the skill: 'The property is handled as CTL'; 'The formula AG EF idle is kept as written'; 'The branching-time meaning (from every reachable state some path reaches idle) is explained'; 'The result is stated to be fairness-free'; 'The answer says whether the witness or counterexample is a path or a tree'
- Eval 7 (starvation-loop-fairness): with_skill 9/9, without_skill 8/9
- Eval 7: 8 of 9 assertions pass in both configurations and do not differentiate the skill: 'Without fairness the property is reported violated'; 'The counterexample is described as a loop that repeats forever, not as a finite trace'; 'Process B is named as the one that is enabled through the loop and never moves'; 'The run under weak fairness is reported and comes back verified'; 'Both results are given: without fairness first, then with weak fairness'; 'Weak fairness is named as an assumption, not as a fact about the system'; 'The engine was invoked (CLI or MCP), not reasoned around'; 'No forbidden phrasing, and the fair result is not called proved'
