# G4 — подтверждение (протокол, п. 6)

Шаг: **G4** (план 14 §9). Критерий выхода: «Совпадение с SPIN на `CH4`, `CH8`, `CH12`, `App_A`; E2, E4 проходят»; в постановке шага уточнено: «вердикты совпадают с SPIN на CH4, CH8, CH12, App_A; контрпримеры для циклов — prefix + loop».

**Соответствие критерию выхода: выполнен** для части «вердикты совпадают с SPIN, контрпримеры prefix + loop» — 43 дифференциальные тройки согласны (§3.1), с оговорками §5: CH12/leader проверен на семантически эквивалентной переписи `leader3.pml` (оригинал вне подмножества G1), а CH4/pcval.pml вне подмножества (`pc_value`). Часть «E2, E4 проходят» принадлежит evals G3/G6 и здесь не проверялась.

## 1. Что сделано

- `features/g4-ltl.feature` — написан до кода (коммит `91813a9`): 31 сценарий, из них outline на 22 строки дифференциальной тройки (`@spin`); словарь фиксирует семантику claim (claim ходит первым; состояние после хода claim не хранится; блокировка claim отсекает путь; конец claim и падающий `assert` claim — нарушения на конечном префиксе; atomic внутри claim — один шаг claim; stutter extension), форму контрпримера (`loop.start`, `loop.steps`), правила статусов и бюджетов. Правки по ходу шага (все — до закрытия): направление отрицания у App_A, счётчики 4 вместо pan-овских 5 на fairness.pml/fair.pml (объяснено), модель для сценария time budget, сценарий `mc_parse` с Promela (поручение координатора), stutter extension.
- `engine/ltl/` — парсер SPIN-синтаксиса (`[] <> U V X ! && || -> <->`, `/\ \/`, `true/false`, атомы: идентификаторы, скобочные сравнения и арифметика, `a[i]`, `len/empty/nempty/full/nfull(ch)`), раскрытие объектных `#define` на уровне токенов с защитой от рекурсии, проверка атомов по `ir.Scope`; NNF (`->`, `<->` устраняются); tableau GPVW (узлы Incoming/New/Old/Next, детерминированный порядок по строковому ключу формулы), обобщённый автомат Büchi с одним множеством на каждую U/`<>`-подформулу, дегенерализация счётчиком по источнику, упрощения (недостижимые, не ведущие к принимающему циклу — Tarjan, бисимуляционное слияние, удаление поглощаемых переходов), имена состояний как у SPIN (`T0_init`, `accept_S<i>`); лоуринг в claim-процесс IR (`Claim: true`, guard = конъюнкция литералов, `accept` на принимающих локациях); `Info{formula, negated, atoms, stutter_invariant, размеры}`. Формула с `X` → `stutter_invariant: false`.
- `engine/explore/cycle.go` — произведение и вложенный DFS: claim-шаг (с atomic-продолжением), затем системный ход; хранятся только состояния после системного хода (как у pan — совпадение счётчиков §3.2); правило хранения `atomic` из G1 сохранено (claim не ходит на промежуточных состояниях); `end`-локация claim = завершение claim = нарушение; `assert` в claim = нарушение на префиксе; принимающие состояния — `accept` у claim, а без claim — у любого процесса; вложенный DFS с замыканием на стеке внешнего поиска; stutter extension (§3.1, находка 2 ревью); non-progress через синтезированный `np_`-автомат pan (`s0: np_→s1 | true→s0; s1(accept): np_→s1`, `np_` = никто не на progress-метке); weak fairness — n+2 копий (байт копии в векторе, null-шаги; принимающие — только копия 0); strong fairness → `not-executed` с причиной FR-008; отдельные `Stats` и `TemporalInfo` на свойство; бюджеты: states/depth → `bounded`, time/memory → `unknown` (план §6, применено и к безопасностному поиску); `FormulaError` → отказ входа (kind `ltl`).
- `engine/explore/explore.go` — `Run` оркестрирует безопасностный поиск и по одному произведению на ltl/progress-свойство; `Options{Fairness, Defines}`; implicit `assert` не включает assert'ы claim; `budgetEvidence`.
- `engine/cex` — `Loop{start, steps}`, `BuildLasso`, `Prefix()/LoopSteps()`, null-шаги (процесс `-`) для fairness и stutter; summary `prefix; loop: …`.
- `engine/report` — `temporal{source, formula, negated, atoms, stutter_invariant, automaton_*, fairness, claim}`, счётчики и `complete` на свойство, `inconclusive` с `bounded` или `unknown`.
- `engine/frontend/promela` — `end` на выходной локации claim; свойства `never` (есть claim) / `accept` (accept-метки без claim) / `progress` (progress-метки) добавляются фронтендом; предупреждение «never claim … not executed» снято; `Result.Defines` (объектные макросы и `-D`), `PreprocessMacros`.
- `engine/cli` — `--ltl` (повторяемый; ids `ltl1…`), `--progress`, `--fairness none|weak|strong`, `--unlimited`; бюджетные флаги: отсутствие или `0` = умолчание (план §6 как изменён), `--unlimited` снимает всё, отчёт показывает 0; `ParsePromela` общий с сервером; отказ по формуле — exit 2, kind `ltl`.
- `engine/mcp` — `mc_check`: поле `formula` у свойств ltl (`expr` для ltl/progress отвергается), `fairness strong` принимается и даёт `not-executed`, `#define`-таблица сессии для атомов, отказ по формуле как `rejected{kind: ltl}`, `temporal` и `loop` в ответе; `mc_explain`: `prefix`/`loop` по `loop.start`, `loop_note`; `mc_lint_property`: kind `ltl` с `formula` (атомы, неопределённые, `x_free`, `nnf`, класс safety/liveness по синтаксису NNF, заметка о вакуумности антецедента); **поручение координатора**: `mcp.PromelaViaCLI` и его подключение в `cmd/mcd/serve.go` (`Config.Promela`), `mc_parse{promela}` возвращает IR с предупреждениями фронтенда.
- `engine/tools/pandiff/triple.go` + `cmd/pandiff -mode a|l [-ltl] [-fairness]` — тройка (движок; движок с claim от `spin -f '!(φ)'` при удалённом claim модели; pan `-a|-l [-f] -c0`), `#define` в копии для pan (`-D` через shell у spin ломается на скобках); свёртка `assert` только для моделей без claim «как написано» (ревью, находка 6).
- `engine/testdata/promela/starvation.pml` (два процесса, голодание B), `leader3.pml` (CH12/leader для N = 3 без конструкций вне подмножества); `engine/testdata/golden/petrinet2.report.json` — только `memory_bytes_est` (кадр DFS 48 → 64 байт, как в G1).
- `engine/steps_g4_test.go`; правки чужих прежних файлов в `engine/` (мои по владению волны): `steps_g1_test.go` (два шага под изменённые сценарии), `steps_g2_test.go` (текст `loop_note`), `explore_test.go` (`ctl` вместо `ltl` как пример неисполняемого kind), `promela_test.go` (claim теперь свойство `never`). Feature-файлы прежних шагов изменены минимально и с пометкой «Amended in G4»: `g0-engine.feature` (time budget → evidence `unknown`), `g1-promela.feature` (claim — свойство `never`, `end` на выходе claim), `g2-mcp.feature` (ltl/progress исполняются; пример неисполняемого kind — `ctl`; `loop_note`).
- `steps/g4-logika.md` — ревью: 14 находок (6 критичных), все критичные исправлены.
- Зависимости не добавлялись; `skills/`, `evals-workspace/`, `steps_g3_test.go`, `features/g3-*` не тронуты.

## 2. Тесты

```
gofmt — чисто; go vet ./... — чисто
go test ./...                  — ok: ltl, explore, cex, report, ir, cli, mcp, frontend/*, tools/pandiff
  новые юнит-тесты G4: ltl 9 (прецеденты, #define, ошибки, scope, NNF, язык на всех лассо
  ≤ 3 для 23 формул + 150 случайных против brute-force семантики, размеры автоматов,
  ForProperty), explore 8 (корпус 11 случаев с числами pan, fair -l, 16 формул на корпусе,
  starvation none/weak/strong, leader3 = 1340, hand-built nested DFS + accept stutter +
  бюджет, budgetEvidence, FormulaError), mcp 1 (lint ltl), pandiff 2 (43 тройки ≈ 28 с,
  stripNever)
godog (общий harness, -count=1): 204 scenarios: 203 passed, 1 failed; 1143 steps: 1139 passed
  — g4-ltl.feature: 31 сценарий (outline 22 строки), все PASS (@spin — при установленном spin);
  — 1 failed: g3-evals.feature «engine-tools reference names every flag» — references/engine-tools.md
    (файл агента G3) не знает новых флагов --ltl --progress --fairness --unlimited; см. §7.
```

## 3. Замеры

Машина: linux/arm64, Go 1.26; SPIN 6.5.2, `spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE [-DNP]`, `./pan -a|-l [-f] -c0`; 2026-09-25; машина разделялась с другими агентами.

### 3.1. Дифференциальные тройки (`TestDifferentialTriples`, `features/g4-ltl.feature` outline)

Формат: движок (число состояний произведения) | движок с claim от `spin -f` (число состояний) | pan (stored, класс первой ошибки). Формула пустая = «модель как написана» (`pan -a` с её claim / accept-метками; `-l` — non-progress).

| модель | формула | fairness | режим | движок | движок + SPIN claim | pan | итог |
|---|---|---|---|---|---|---|---|
| CH4/prop.pml | `[]p` | none | a | violated (5) | violated (3) | violated (3, assertion violated) | agree |
| CH4/prop.pml | `<>[]p` | none | a | violated (5) | violated (5) | violated (7, acceptance cycle) | agree |
| CH4/prop.pml | `[]<>p` | none | a | violated (5) | violated (5) | violated (5, acceptance cycle) | agree |
| CH4/prop.pml | `<>!p` | none | a | violated (3) | violated (3) | violated (3, acceptance cycle) | agree |
| App_A/example | `<>[]p` | none | a | violated (8) | violated (8) | violated (8, acceptance cycle) | agree |
| App_A/example | `[]<>p` | none | a | verified (8) | verified (8) | verified (8) | agree |
| App_A/example | `[]<>!p` | none | a | verified (10) | verified (10) | verified (10) | agree |
| App_A/example | `X p` | none | a | violated (8) | violated (3) | violated (3, assertion violated) | agree |
| App_A/example | `X X p` | none | a | verified (3) | verified (3) | verified (3) | agree |
| App_A/example | `p U (x == 1)` | none | a | violated (11) | violated (5) | violated (5, assertion violated) | agree |
| App_A/example | (claim модели) | none | a | verified (10) | n/a | verified (10) | agree |
| CH8/trivial.pml | (claim модели) | none | a | violated (2) | n/a | violated (2, acceptance cycle) | agree |
| CH8/trivial.pml | (claim модели) | weak | a | violated (6) | n/a | violated (2, acceptance cycle) | agree |
| CH8/trivial.pml | `[]<>x` | none | a | verified (3) | verified (3) | verified (3) | agree |
| CH8/trivial.pml | `[]<>x` | weak | a | verified (5) | verified (5) | verified (3) | agree |
| CH8/trivial.pml | `[]<>!x` | weak | a | verified (4) | verified (4) | verified (3) | agree |
| CH8/fairness.pml | (accept-метки) | none | a | violated (4) | n/a | violated (5, acceptance cycle) | agree |
| CH8/fairness.pml | (accept-метки) | weak | a | violated (16) | n/a | violated (4, acceptance cycle) | agree |
| CH8/example.pml | (как написана) | none | a | violated (assert) (6) | n/a | violated (6, assertion violated) | agree |
| CH4/fair_accept.pml | (accept-метки) | none | a | violated (4) | n/a | violated (4, acceptance cycle) | agree |
| CH4/fair_accept.pml | (accept-метки) | weak | a | violated (16) | n/a | violated (5, acceptance cycle) | agree |
| CH4/fair.pml | `[]<>(x == 1)` | none | a | verified (3) | verified (3) | verified (3) | agree |
| CH4/fair.pml | `[]<>(x == 1)` | weak | a | verified (4) | verified (4) | verified (3) | agree |
| CH4/fair.pml | `<>[](x == 1)` | none | a | violated (3) | violated (3) | violated (4, acceptance cycle) | agree |
| CH4/fair.pml | (np_) | none | l | violated (4) | n/a | violated (5, non-progress cycle) | agree |
| CH4/fair.pml | (np_) | weak | l | violated (10) | n/a | violated (4, non-progress cycle) | agree |
| CH4/dijkstra_progress.pml | (np_) | none | l | verified (39) | n/a | verified (39) | agree |
| CH4/dijkstra_progress.pml | (np_) | weak | l | verified (99) | n/a | verified (39) | agree |
| CH4/dijkstra_progress.pml | `[]<>(len(sema) == 0)` | none | a | verified (21) | verified (21) | verified (21) | agree |
| CH4/true.pml | `[]true` | none | a | verified (1) | verified (1) | verified (1) | agree |
| CH4/false.pml | `[]true` | none | a | verified (1) | verified (1) | verified (1) | agree (см. §5: assert вне области claim) |
| CH3/alternatingbit.pml | `[] (full1 -> <> empty1)` | none | a | verified (10) | verified (10) | verified (10) | agree |
| CH3/alternatingbit.pml | `[] (full1 -> <> empty1)` | weak | a | verified (14) | verified (14) | verified (10) | agree |
| CH3/alternatingbit.pml | `[]<> two` (`len(to_rcvr) == 2`) | none | a | violated (16) | violated (16) | violated (16, acceptance cycle) | agree |
| starvation.pml | `<>done` | none | a | violated (5) | violated (4) | violated (4, acceptance cycle) | agree |
| starvation.pml | `<>done` | weak | a | verified (11) | verified (10) | verified (4) | agree |
| starvation.pml | `done U (done)` | none | a | violated (11) | violated (4) | violated (4, assertion violated) | agree |
| starvation.pml | `[]!done` | none | a | violated (10) | violated (6) | violated (6, assertion violated) | agree |
| leader3.pml (CH12) | `<>[]oneLeader` | none | a | verified (1340) | verified (1340) | verified (1340) | agree |
| leader3.pml (CH12) | `<>[]oneLeader` | weak | a | verified (7443) | verified (7442) | verified (1340) | agree |
| leader3.pml (CH12) | `[]noLeader` | none | a | violated (697) | violated (679) | violated (679, assertion violated) | agree |
| leader3.pml (CH12) | `<>elected` | none | a | verified (662) | verified (662) | verified (662) | agree |
| leader3.pml (CH12) | `<>elected` | weak | a | verified (3523) | verified (3464) | verified (662) | agree |

Итого 43 тройки, 43 agree. Число состояний совпадает с pan, когда сравнимо (см. §3.2); под `-f` числа pan не сравниваются (у pan копии не входят в «stored»), под собственным автоматом движка число состояний зависит от автомата (у SPIN другие упрощения) и тоже не сравнивается. Класс pan «assertion violated» на claim'ах `spin -f` — это assert внутри claim (safety-часть формулы); движок с тем же claim даёт «assertion violated in the never claim», со своим автоматом — acceptance cycle: вердикт тот же, класс различен по замыслу.

По ходу шага тройки выявили три расхождения, все исправлены (ревью, находки 2, 3, 6): atomic внутри claim (`X p`), stutter extension (`[]noLeader`), свёртка assert (`false.pml`).

### 3.2. Число состояний произведения против pan (`states, stored`, без fairness)

| модель | свойство | pan | движок | сверка |
|---|---|---|---|---|
| CH4/prop.pml `-D PHI` | never (`[]p`) | 3 | 3 | точно |
| CH4/prop.pml | never (`![]p`, конец claim) | 5 | 5 | точно (pan продолжает после «end state in claim», хранит состояния с завершившимся claim; движок в `--sweep` — так же) |
| App_A/example | never | 10 | 10 | точно |
| CH8/trivial.pml | never | 2 | 2 | точно |
| CH4/fair_accept.pml | accept | 4 | 4 | точно |
| CH4/dijkstra_progress.pml | progress (`-l`) | 39 | 39 | точно |
| leader3.pml + CH12/leader.ltl | never | 1340 | 1340 | точно |
| leader4 (N = 4, scratch) | never | 10 299 | 10 299 | точно |
| leader5 (N = 5, scratch) | never | 83 370 | 83 370 | точно |
| CH8/fairness.pml | accept | 5 | 4 | объяснено: `pan -DCHECK` показывает «New state 3+», «New state 1+» — повторные вставки вложенного поиска; plain `pan -c0` даёт 4 |
| CH4/fair.pml | progress (`-l`) | 5 | 4 | то же (np_-произведение 2 × 2; plain 2) |

Правило, найденное пробами и подтверждённое числами: pan не хранит состояние после хода claim; хранимые состояния — пары (состояние системы, локация claim) после системного шага. Правило хранения `atomic` (G1) в произведении сохранено (claim не ходит внутри атомарной последовательности).

### 3.3. Производительность на модели leader (с claim CH12/leader.ltl, `-a`, полный обход)

| N | состояния (pan = движок) | pan `-a -c0` (wall, RSS) | движок `--sweep --unlimited` (wall, RSS; `time_ms` свойства never) |
|---|---|---|---|
| 3 | 1 340 | 0,02 с, 140 МБ | 0,00 с, 19 МБ (8 мс) |
| 4 | 10 299 | 0,16 с, 582 МБ (`-w26`) | 0,02 с, 32 МБ (15 мс) |
| 5 | 83 370 | 0,38 с, 596 МБ (`-w26`) | 0,18 с, 95 МБ (148 мс) |

Wall-время pan включает выделение хеш-таблицы `-w26`, поэтому это порядок величины, не бенчмарк: на N = 5 движок не медленнее pan; условие K1 («< 50× медленнее pan») выполнено. Под `--fairness weak` на N = 3: 7 442 состояния произведения за 8 мс. Юнит-набор тройки (43 прогона spin + gcc + pan + движок) — 28 с, в нём доминируют gcc-компиляции pan.

## 4. Решения, зафиксированные в коде и требующие внимания владельца

1. **Семантика claim** (одна формулировка — `cycle.go`, продублирована словарём feature): claim ходит первым; промежуточное состояние не хранится; блокировка отсекает; `end`-локация claim = завершение = нарушение (SPIN «end state in claim reached»); assert в claim = нарушение; atomic в claim — один шаг claim; stutter extension по умолчанию (как у pan; аналога `-DNOSTUTTER` нет).
2. **Направление отрицания**: свойство `ltl` с `formula` φ проверяется автоматом для `!(φ)`; `temporal.negated` печатает его; свойство `never` без формулы — «claim модели не принимает ни одного прогона» (что бы ни говорил комментарий claim'а; см. App_A).
3. **Свойства фронтенда**: `never` при claim, иначе `accept` при accept-метках; `progress` при progress-метках (pan -l — по требованию; здесь — автоматически, одно условие в `lower.go`, если владелец хочет иначе).
4. **Assert и claim**: движок проверяет assert безопасностным поиском по всему пространству; pan `-a` — только в области claim. Движок строже; тройка сворачивает assert только для моделей без claim.
5. **Weak fairness**: копии n+2 с null-шагами (процесс `-` в трассе); принимающие — только копия 0; claim и stutter не считаются процессами. Вердикты совпали с `pan -f` на 11 строках; числа состояний не сравниваются.
6. **Бюджеты** (план §6 как изменён): CLI — отсутствие или 0 = умолчание; `--unlimited` — 0 в отчёте; MCP без изменений; evidence `bounded` для states/depth, `unknown` для time/memory — применено и к безопасностному поиску (сценарий G0 исправлен с пометкой).
7. **Отчёт**: `search.complete` — полнота безопасностного поиска; temporal-свойства несут свои `counters`/`complete` (`report.Search` doc).
8. **Ветвление внутри atomic claim** — первая разрешённая альтернатива; блокировка atomic claim на середине — claim остаётся (pan не пробовался, в сгенерированных claim'ах недостижимо).
9. **`mc_lint_property` для ltl** — класс safety/liveness по синтаксису NNF (без U/`<>` → safety), заметка о вакуумности антецедента; это подсказка, не классификация по семантике.
10. Формулы: `U`, `V`, `X` — всегда операторы (как в SPIN); функциональные `#define` в формулах не раскрываются.

## 5. Отклонения, отложено, K3

- **CH12/leader вне подмножества G1** (массив каналов `chan q[N]`, канальные параметры `chan in, out`, `run` в цикле `do`) — план относит каналы в сообщениях к G5. Оракул выполнен на `testdata/promela/leader3.pml`: та же программа с раскрученным `run`, именованными каналами и подставленными параметрами (числа инстансов `(N+I-proc)%N+1` как в оригинале); pan и движок дают одинаковые числа состояний при N = 3, 4, 5. Утверждение «совпадение с SPIN на CH12» относится к этой переписи. Рекомендация: включить массивы каналов и канальные параметры в G5 вместе с каналами в сообщениях, тогда оракул повторяется на самом `CH12/leader` (`N=7` без редукций у pan будет большим — понадобится `-D N=…`, что оригинал поддерживает).
- **CH4/pcval.pml** — `pc_value()` вне подмножества (G1); тройки нет.
- **Постановка шага про alternatingbit** («violated without fairness with loop naming the stuttering process») не выполнима на CH3/alternatingbit.pml: протокол lock-step (8 состояний, один цикл, ни одного выбора), живость там не зависит от fairness — pan `-a` и `-a -f` согласны (§3.1). Случай голодания покрыт `starvation.pml` (violated/loop только A без fairness, verified с weak, как у pan).
- **Счётчики pan под `-a`/`-l`** на моделях с принимающим начальным состоянием включают повторную вставку вложенного поиска (§3.2); движок сообщает число состояний произведения.
- **Assert в области claim** (§4, п. 4) — движок строже pan; в тройках свёртка ограничена.
- Не измерено: pan для claim с блокирующейся atomic-последовательностью; формулы с ≥ 3 атомами и лассо > 3 в проверке языка (только случайные формулы над двумя атомами).
- **K3** (план §9): LTL-часть сошлась с оракулом (43/43) — контингентное решение «never claim от внешнего инструмента как единственный LTL-вход» не требуется. Доля мутантов и второй корпус в этом шаге не измерялись (мутационные тесты §8.1 не строились — не входили в постановку G4). Трудозатраты: одна сессия против оценки 15–22 pd.
- Отложено: `-DNOSTUTTER`-режим; сравнение чисел состояний под weak fairness; lint CTL (G5); POR-запрет по `stutter_invariant` (vNext).

## 6. Соответствие критерию выхода 14 §9 (строка G4)

| Часть критерия | Свидетельство | Статус |
|---|---|---|
| Совпадение с SPIN на CH4 | prop.pml (обе ветви `#ifdef PHI`, 4 формулы), true/false, fair (`-a`, `-l`, `-f`), fair_accept (`-a`, `-f`), dijkstra_progress (`-l`, `-l -f`, формула); pcval — вне подмножества | выполнено (кроме pcval, §5) |
| … на CH8 | fairness (`-a`, `-f`), trivial (claim, `-f`, 3 формулы), example (assert) | выполнено |
| … на CH12 | leader3 + CH12/leader.ltl: `<>[]oneLeader` (claim = автомат = pan, 1340 состояний), `[]noLeader`, `<>elected`, weak; leader4/5 счётчики | выполнено на переписи (§5) |
| … на App_A | claim модели, `<>[]p`, `[]<>p`, `[]<>!p`, `X p`, `X X p`, `p U (x == 1)` | выполнено |
| Контрпримеры циклов — prefix + loop | `cex.Loop{start, steps}` в CLI JSON и `mc_check`/`mc_explain` (`prefix`/`loop`); сценарии «has a loop», «loop is closed», «only A:0 or the claim», mc_explain | выполнено |
| E2, E4 проходят | evals G3/G6 (skills/, evals-workspace/ — не мои файлы) | не проверялось здесь |

## 7. Для агента G3 (документация и evals)

Новое в интерфейсах, которое надо отразить в `references/engine-tools.md`, SKILL.md (шаг 3) и сценарии G3 «does not link the Promela frontend» (теперь фронтенд подключён):
- `mcd check`: `--ltl 'формула'` (повторяемый; свойства `ltl1`, `ltl2`, …), `--progress`, `--fairness none|weak|strong`, `--unlimited`; бюджетные флаги: 0/отсутствие = умолчание (1e6 / 1e6 / 60 000 мс / 1024 МиБ); отказ по формуле — exit 2, `error.kind: "ltl"`.
- Отчёт: `properties[].temporal{source, formula, negated, atoms, stutter_invariant, automaton_states, automaton_transitions, automaton_accepting, fairness, claim}`, `counterexample.loop{start, steps}`, шаги процесса `never…` (claim) и `-` (null/stutter); `inconclusive` с evidence `unknown` для time/memory; `search.complete` — безопасностный поиск, у temporal-свойств свой `complete`.
- Свойства модели из Promela: `never` / `accept` / `progress` добавляются фронтендом.
- MCP: `mc_parse{promela}` работает (`outcome: ir`, `warnings` фронтенда); `mc_check`: `properties[].formula` для `ltl` (без `expr`), `fairness: strong` → `not-executed`, `temporal` и `counterexample.loop` в ответе, `rejection.kind: "ltl"`; `mc_explain`: `prefix`/`loop`/`loop_note`; `mc_lint_property`: `kind: "ltl"` с `formula`.
- Сценарий G3 «engine-tools reference names every flag» сейчас красный из-за трёх новых флагов — правка на стороне G3.

## 8. Файлы

- `model-check-plugin/features/g4-ltl.feature`; правки `features/g0-engine.feature`, `g1-promela.feature`, `g2-mcp.feature`
- `model-check-plugin/engine/ltl/{ltl,buchi,claim,ltl_test}.go`
- `model-check-plugin/engine/explore/{cycle,cycle_test,explore,explore_test}.go`
- `model-check-plugin/engine/{cex/cex.go, report/report.go, ir/ir.go, cli/cli.go}`
- `model-check-plugin/engine/frontend/promela/{lower,preproc,promela,promela_test}.go`
- `model-check-plugin/engine/mcp/{check,lint,parse,server,session}.go`, `mcp/lint_g4_test.go`
- `model-check-plugin/engine/cmd/mcd/serve.go`, `engine/cmd/pandiff/main.go`
- `model-check-plugin/engine/tools/pandiff/{triple,triple_test}.go`
- `model-check-plugin/engine/testdata/promela/{starvation,leader3}.pml`, `engine/testdata/golden/petrinet2.report.json`
- `model-check-plugin/engine/{steps_g4_test,steps_g1_test,steps_g2_test}.go`
- `model-check-plugin/steps/g4-logika.md`, `model-check-plugin/steps/g4-confirmation.md`

Коммиты: `91813a9` (feature до кода), `3836a67` (реализация), плюс коммит этого подтверждения и ревью.
