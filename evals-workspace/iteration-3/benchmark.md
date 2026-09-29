# Skill Benchmark: model-check

**Model**: claude (same model as the coordinating session)
**Date**: 2026-09-25
**Evals**: 1, 2, 3, 4, 5, 7 (1 run(s) each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|------------|---------------|-------|
| Pass Rate | 100% ± 0% | 42% ± 7% | +0.58 |
| Time | 452.4s ± 135.1s | 225.1s ± 124.7s | +227.3s |
| Tokens | 142578 ± 18574 | 59796 ± 12968 | +82782 |

## Per eval

| Eval | Configuration | Passed | Pass rate | Time (s) | Tokens | Tool calls |
|------|---------------|--------|-----------|----------|--------|------------|
| 1 mutex-invariant-violated | with_skill | 8/8 | 100% | 434.6 | 137874 | 34 |
| 2 alternating-bit-liveness-fairness | with_skill | 9/9 | 100% | 547.0 | 149919 | 29 |
| 3 petri-net-hang | with_skill | 11/11 | 100% | 407.7 | 149575 | 26 |
| 4 telephone-busy-liveness | with_skill | 10/10 | 100% | 672.2 | 171709 | 29 |
| 5 c-code-out-of-subset | with_skill | 7/7 | 100% | 330.2 | 122931 | 24 |
| 7 starvation-loop-fairness | with_skill | 10/10 | 100% | 322.9 | 123462 | 24 |
| 1 mutex-invariant-violated | without_skill | 3/8 | 38% | 287.0 | 66553 | 21 |
| 2 alternating-bit-liveness-fairness | without_skill | 4/9 | 44% | 376.1 | 67954 | 20 |
| 3 petri-net-hang | without_skill | 5/11 | 45% | 93.3 | 46174 | 3 |
| 4 telephone-busy-liveness | without_skill | 5/10 | 50% | 323.5 | 77655 | 24 |
| 5 c-code-out-of-subset | without_skill | 3/7 | 43% | 198.5 | 54735 | 13 |
| 7 starvation-loop-fairness | without_skill | 3/10 | 30% | 72.4 | 45705 | 7 |

## Notes

- Eval 1 (mutex-invariant-violated): with_skill 8/8, without_skill 3/8
- Eval 1: 3 of 8 assertions pass in both configurations and do not differentiate the skill: 'The counterexample is decoded through the model's labels L1–L4'; 'The counterexample shows both users at label L7 (the critical section)'; 'No forbidden phrasing (no errors / proved / system is correct) (naming the word in order to deny it does not count)'
- Eval 2 (alternating-bit-liveness-fairness): with_skill 9/9, without_skill 4/9
- Eval 2: 4 of 9 assertions pass in both configurations and do not differentiate the skill: 'Fairness is raised as a question or as an explicitly labelled assumption'; 'The formula that was actually checked is shown, in LTL syntax'; 'The claim is limited to this model: it has no message loss, retransmission or timeout'; 'No forbidden phrasing'
- Eval 3 (petri-net-hang): with_skill 11/11, without_skill 5/11
- Eval 3: 5 of 11 assertions pass in both configurations and do not differentiate the skill: 'The counterexample is the firing sequence t1 then t4'; 'The final marking has one token in p2 (p2 = 1, or set notation {p2, p5})'; 'The final marking has one token in p5 (p5 = 1, or set notation {p2, p5})'; 'The number of reachable markings (6) is quoted from the engine's counters'; 'No forbidden phrasing (no errors / proved / net is correct) (naming the word in order to deny it does not count)'
- Eval 4 (telephone-busy-liveness): with_skill 10/10, without_skill 5/10
- Eval 4: 5 of 10 assertions pass in both configurations and do not differentiate the skill: 'It is formalised as a progress label or an LTL eventually/infinitely-often formula'; 'The formula refers to the model's labels or states (Busy), not to printf output'; 'Fairness is raised before the result'; 'The route is named correctly: either the `progress` property the frontend adds from progress labels, or the CTL formula with a control-label atom'; 'No forbidden phrasing (naming the word in order to deny it does not count)'
- Eval 5 (c-code-out-of-subset): with_skill 7/7, without_skill 3/7
- Eval 5: 3 of 7 assertions pass in both configurations and do not differentiate the skill: 'The rejected construct c_code is named'; 'No property result is imitated for the original model: no line of the answer attributes verified/violated/inconclusive to simple1.pr, c_code/c_expr or the original model/file (a verdict about a clearly separate rewrite is allowed — including a line that calls that rewrite "the original" one as against a mutant; refined in iteration-2 and again in iteration-3, see steps/g3-evals3-confirmation.md)'; 'No host code is executed and the answer says so'
- Eval 7 (starvation-loop-fairness): with_skill 10/10, without_skill 3/10
- Eval 7: 3 of 10 assertions pass in both configurations and do not differentiate the skill: 'The counterexample is described as a loop that repeats forever, not as a finite trace'; 'Process B is named as the one that is enabled through the loop and never moves'; 'Weak fairness is named as an assumption, not as a fact about the system'
