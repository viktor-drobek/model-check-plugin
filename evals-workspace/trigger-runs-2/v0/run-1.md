You are the dispatcher of a coding agent. Before answering a user's request you decide
which of the available skills, if any, you would open and read first.

Below is the roster of available skills, then 20 user requests. For each request decide
one thing only: **would you consult the `model-check` skill before answering it?**

Answer for every request. Answer "yes" only if you would actually open that skill for
that request; answer "no" if you would answer without it or would reach for a different
skill. Do not explain, do not hedge, do not answer the requests themselves.

## Available skills

- **claude-api**: Reference for the Claude API / Anthropic SDK — model ids, pricing, params, streaming, tool use, MCP, agents, caching, token counting.
- **code-review**: Review the current diff, or a PR number/branch/path target, for correctness bugs and reuse/simplification/efficiency cleanups.
- **dataviz**: Use whenever you are about to create ANY chart, graph, plot, dashboard or data visualization, in any output medium.
- **docx**: Create, read, edit or manipulate Word documents (.docx) and templates (.dotx).
- **logika**: Классическая формальная логика: проверить корректность рассуждения, найти логические ошибки в тексте, построить силлогизм, оценить индуктивный вывод.
- **model-check**: Model checking of protocols, concurrent algorithms, state machines and Petri nets with a built-in explicit-state engine (Go, exposed as MCP tools mc_parse, mc_check, mc_explain and others, or as the `mcd` CLI) — no external SPIN, NuSMV or other model checker is needed or should be looked for. Use this skill whenever the user wants to verify, prove or refute a behavioural property of a system with several interacting parts: deadlock, mutual exclusion, invariants, reachability, liveness, response, starvation, fairness, LTL, CTL, Promela models, never claims, Petri nets, token nets, alternating-bit or handshake protocols, telephone switches, producer/consumer, leader election, "can this hang", "is a counterexample possible", "verify this protocol", "check this model", "model checking", "deadlock", "counterexample". Russian triggers, use the skill for any of them: «проверь модель», «может ли зависнуть», «докажи, что никогда», «сеть Петри», «тупик», «инвариант», «взаимное исключение», «живость», «liveness», «справедливость», «Promela», «never claim», «LTL», «CTL», «model checking», «верификация протокола», «контрпример», «гонка», «переведи сеть Петри во что-нибудь проверяемое». Also use it when the user only describes the system in words and asks whether something bad can ever happen or something good must eventually happen — that is a model-checking question even if they do not say so. Do not use it for unit tests of sequential code, data-race linters, single-function theorem proving, or for drawing diagrams without analysis.
- **pdf**: Read or extract text and tables from PDFs, merge, split, rotate, watermark, fill forms, OCR scanned PDFs.
- **run**: Launch and drive this project's app to see a change working: run, start or screenshot the app, or confirm a change works in the real app.
- **security-review**: Complete a security review of the pending changes on the current branch.
- **simplify**: Review the changed code for reuse, simplification, efficiency and altitude cleanups, then apply the fixes. Quality only — it does not hunt for bugs.
- **xlsx**: Open, read, edit or create spreadsheets (.xlsx, .csv, .tsv); clean or restructure messy tabular data. The deliverable must be a spreadsheet file.

## Requests

1. у меня два процесса дерутся за мьютекс, можешь доказать что они не зайдут одновременно в критическую секцию? файл в CH2/mutex_flaw.pml
2. help me finish this Coq proof that my merge function is correct — the Gallina file is proofs/Merge.v
3. refactor this state machine class to use an enum instead of magic strings, and keep the tests passing
4. запиши свойство «каждый запрос рано или поздно обслуживается» формулой LTL и проверь её на моей модели в server.pml, с допущением справедливости и без него
5. add Promela syntax highlighting to my editor config and make .pml files open with it
6. у меня сеть Петри: места p1..p6, переходы t1..t6 с дугами как в схеме. переведи её во что-нибудь проверяемое и скажи, может ли она зависнуть
7. что такое model checking и чем оно отличается от тестирования? нужен короткий обзор на две минуты для лекции
8. переведи главу про never claims из документации SPIN на русский, сохранив примеры кода
9. can this handshake deadlock if the ack is lost? the protocol is in CH3/alternatingbit.pml
10. SPIN собирается медленно и pan съедает 6 ГБ на моей машине — как ускорить сборку и уменьшить расход памяти самой программы?
11. проверь, что телефонный коммутатор из CH14/version1 не может навсегда застрять в состоянии Busy
12. I have a leader election protocol in CH9/leader.pml — does exactly one leader eventually get elected, or can the election run forever?
13. draw me a Petri net diagram of our order pipeline for tomorrow's slides — just the picture, no analysis
14. write unit tests for my concurrent queue in internal/queue/queue.go, including a few goroutine stress tests
15. prove that the shared counter in internal/worker/counter.pml can never go negative when two writers run concurrently
16. two of our services take locks A and B in the opposite order; here is the pseudocode — can they end up waiting for each other forever?
17. is AG EF idle true for the switch model in CH14/version1, or can it reach a state from which idle is unreachable?
18. мой вариант алгоритма взаимного исключения нарушает безопасность, а peterson.pml — нет. дай контрпример, который показывает, где ломается мой
19. профилируй мой Go-сервис: он проводит 80 % времени в сборщике мусора, найди, где столько аллокаций
20. запусти go test -race ./internal/worker и объясни, что именно нашёл детектор гонок

## Output

Write a JSON file to `${REPO_ROOT}/model-check-plugin/evals-workspace/trigger-runs-2/v0/run-1.json` and nothing else. Its content must be exactly a JSON object
mapping each request number (as a string) to "yes" or "no", for example:

{"1": "no", "2": "yes", "3": "no"}

Include every request number from 1 to 20. Then reply with just the word DONE.
