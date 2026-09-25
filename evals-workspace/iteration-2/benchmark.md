# Skill Benchmark: model-check

**Model**: claude-fable-5-1 (subagents of the coordinating session)
**Date**: 2026-09-25T12:00:00Z
**Evals**: 1, 3, 5 (1 run(s) each per configuration)

## Summary

| Metric | With Skill | Without Skill | Delta |
|--------|------------|---------------|-------|
| Pass Rate | 100% ± 0% | 57% ± 12% | +0.43 |
| Time | 281.5s ± 37.2s | 98.8s ± 57.9s | +182.8s |
| Tokens | 111142 ± 10494 | 49298 ± 7535 | +61844 |

## Per eval

| Eval | Configuration | Passed | Pass rate | Time (s) | Tokens | Tool calls |
|------|---------------|--------|-----------|----------|--------|------------|
| 1 mutex-invariant-violated | with_skill | 8/8 | 100% | 287.2 | 116107 | 17 |
| 3 petri-net-hang | with_skill | 10/10 | 100% | 315.6 | 118231 | 21 |
| 5 c-code-out-of-subset | with_skill | 7/7 | 100% | 241.9 | 99087 | 11 |
| 1 mutex-invariant-violated | without_skill | 4/8 | 50% | 88.5 | 49317 | 6 |
| 3 petri-net-hang | without_skill | 5/10 | 50% | 46.7 | 41754 | 2 |
| 5 c-code-out-of-subset | without_skill | 5/7 | 71% | 161.1 | 56823 | 13 |

## Notes

- Eval 1 (mutex-invariant-violated): with_skill 8/8, without_skill 4/8
- Eval 1: 4 of 8 assertions pass in both configurations and do not differentiate the skill: 'The mutual-exclusion property is reported with status violated'; 'The counterexample is decoded through the model's labels L1–L4'; 'The counterexample shows both users at label L7 (the critical section)'; 'No forbidden phrasing (no errors / proved / system is correct)'
- Eval 3 (petri-net-hang): with_skill 10/10, without_skill 5/10
- Eval 3: 5 of 10 assertions pass in both configurations and do not differentiate the skill: 'The counterexample is the firing sequence t1 then t4'; 'The final marking has one token in p2 (p2 = 1, or set notation {p2, p5})'; 'The final marking has one token in p5 (p5 = 1, or set notation {p2, p5})'; 'The number of reachable markings (6) is quoted from the engine's counters'; 'No forbidden phrasing (no errors / proved / net is correct)'
- Eval 5 (c-code-out-of-subset): with_skill 7/7, without_skill 5/7
- Eval 5: 5 of 7 assertions pass in both configurations and do not differentiate the skill: 'Status not-executed is reported'; 'The rejected construct c_code is named'; 'A source line of the construct is given'; 'No property result is imitated for the original model: no line of the answer attributes verified/violated/inconclusive to simple1.pr, c_code/c_expr or the original (a verdict about a clearly separate rewrite is allowed; iteration-2 refinement, see evals-workspace/iteration-2)'; 'No host code is executed and the answer says so'
