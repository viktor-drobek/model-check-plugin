# Согласование ревью — Codex (gpt-6-sol)

Дата: 2026-09-29. Инструмент: `codex exec -m gpt-6-sol -s read-only -C <repo> --json`,
отдельная сессия (не тред astra), чтобы вердикты были независимыми.

Предмет: 19 находок §E и находки разделов A–D из
[review-astra-skill.md](review-astra-skill.md). Задание требовало не пересказа, а
самостоятельной проверки каждой находки по первоисточникам (skill, `engine/`,
`features/`, `steps/*-confirmation.md`, заметки 11 и 14), собственных ссылок
файл:строка и явного указания случаев, где ссылка первого рецензента не
подтверждается.

Сокращения в ссылках рецензента: `S/` = `model-check-plugin/skills/model-check/`,
`G/` = `model-check-plugin/engine/`, `F/` = `model-check-plugin/features/`,
`C/` = `model-check-plugin/steps/`, `N/` = `model-check-skill-notes/`.

Итог: из 19 находок §E — 15 ПОДТВЕРЖДЕНО, 4 ЧАСТИЧНО, 0 ОТКЛОНЕНО; все 16 находок
разделов A–D подтверждены. Ошибочных ссылок у первого рецензента не найдено.
Четыре находки добавлены сверх его списка (§3), четыре его вывода сужены (§4).

Ниже — ответ рецензента дословно.

---

## 1. Вердикты по §E

| № | Краткая суть | Вердикт | Чем подтверждается/опровергается | Где правка |
|---|---|---|---|---|
| 1 | `bounded` обещает покрытие до глубины D | ПОДТВЕРЖДЕНО | `S/references/evidence-and-status.md:92` — «everything up to depth D»; `G/explore/explore.go:1505` — `if !isNew { continue }`, `:1520` — состояние глубже D не раскрывается; `:241` — такой stop всё равно даёт `Bounded`. | и то и другое |
| 2 | Инвертирована регрессия `reach bad` | ПОДТВЕРЖДЕНО | `S/references/counterexamples.md:178–180` — сохранить `reach`, затем ожидать `verified`; `G/explore/explore.go:1765–1767` — при полном поиске без такого состояния результат `Violated`. | текст skill |
| 3 | `mc_explain` назван replay | ПОДТВЕРЖДЕНО | `S/references/workflow.md:120` — «`mc_explain` replayed»; `G/mcp/check.go:460–467` — чтение и декодирование сохранённого JSON; `G/mcp/simulate.go:30–31` — guided mode принимает `edges`, а не ID. | текст skill |
| 4 | Evals засчитывают упоминание вызова как вызов | ПОДТВЕРЖДЕНО | `S/evals/evals.json:50–53` — шаблон `mcd check\|mc_check`; `model-check-plugin/evals-workspace/grader.py:46–49` — `re.search` только по ответу. Сам результат 59/59 я не воспроизводил; структуру 59 проверок проверил чтением JSON: 48 regex, 8 not_regex, 2 regex_order, 1 petri_json_valid. | evals |
| 5 | Skill запрещает действующий CTL | ПОДТВЕРЖДЕНО | `S/SKILL.md:266` — «does not check CTL yet»; `F/g5-ctl-v1.feature:105–114` — `AG EF subscriber@Idle` даёт `verified`/`exhaustive`. | и то и другое |
| 6 | Обещано CTL-дерево с разбором ветвей | ПОДТВЕРЖДЕНО | `S/references/counterexamples.md:32` — `mc_explain` объяснит «per branch»; `G/ctl/label.go:265` — «why the nested subformula fails … is not shown». | текст skill |
| 7 | `mc_explain` обязателен для каждого `violated` | ПОДТВЕРЖДЕНО | `S/SKILL.md:174` — «For every `violated`»; `G/ctl/label.go:258` — у ложного existential нет run; `G/explore/explore.go:1765–1767` — недостижимый `reach` нарушен без трассы. | текст skill |
| 8 | Явный список свойств удаляет автоматические | ПОДТВЕРЖДЕНО | `S/references/properties-ltl-ctl.md:214–217` — `progress` якобы есть в каждом отчёте; `G/mcp/check.go:214–217` — `mm.Properties = props`; исключение для implicit `assert` указано в `:47`. | текст skill |
| 9 | Ручной вывод при strong fairness выглядит как статус | ЧАСТИЧНО | `S/references/fairness.md:143–144` — «`violated` for the strong-fairness reading» при `G/explore/cycle.go:285–288` — `NotExecuted`. Однако `S/references/fairness.md:130` прямо велит сохранять поле статуса движка. Ручной вывод о конкретном лассо возможен; обозначение его статусом двусмысленно. | текст skill |
| 10 | Ошибка входа названа `invalid-model` | ПОДТВЕРЖДЕНО | `S/SKILL.md:209` — «undefined atom» в `invalid-model`; `G/cli/cli.go:345–353` — undeclared variable в свойстве отклоняется с `not-executed`. | текст skill |
| 11 | Даны отвергаемые CTL-примеры | ПОДТВЕРЖДЕНО | `S/references/properties-ltl-ctl.md:48,74` — `proc[i]@label`, `E(p U q)`; `G/ctl/ctl.go:483–491` — until требует `[ … U … ]`, `:585–592` — remote разбирает `P@label` или `P:pid@label`. | текст skill |
| 12 | Petri `fire(t)` представлен как исполнимая формула | ЧАСТИЧНО | `S/references/petri-nets.md:133–145` одновременно помечает `AG EF fire(t)` как G5 и говорит «neither can be run»; `G/frontend/petri/petri.go:229–245` строит guards и переходы, но не атом события. Буквальный `fire(t)` действительно не исполняется; проверка по `enabled(t)` возможна как описанный в `N/14-skill-building-plan.md:147` CTL-эквивалент и LTL-прокси. | и то и другое |
| 13 | Lint обещает анализ полярности never claim | ПОДТВЕРЖДЕНО | `S/references/properties-ltl-ctl.md:157` — «polarity note»; `G/mcp/lint.go:60–75` принимает лишь invariant/reach/ltl/ctl и требует формулу для ltl. | текст skill |
| 14 | Manifest якобы принимает карточку, mapping и свойства | ПОДТВЕРЖДЕНО | `S/references/workflow.md:105` — «All are inputs to `mc_manifest`»; `G/mcp/check.go:501–504` — вход manifest содержит только `session_id`; `G/mcp/session.go:249–257` — параметры вызова не содержат выражений свойств. | и то и другое |
| 15 | Проверка платформ изложена без границы | ЧАСТИЧНО | `S/references/engine-tools.md:311` совмещает cross-platform build и «validated with a real client»; `C/g6-confirmation.md:29–37` отмечает запуск только linux/arm64, а `:509–510` — отсутствие запуска Windows/macOS. `model-check-plugin/mcp/servers.json:5` выбирает POSIX `engine/bin/mcd` (`:1` — `#!/bin/sh`). Из этого нельзя без запуска заключить, что нативный Windows-клиент точно сломан. | текст skill |
| 16 | `--no-timing` обещает побайтную стабильность даже при timeout | ПОДТВЕРЖДЕНО | `S/references/engine-tools.md:275–278` — «byte-for-byte equal»; `G/cli/cli.go:338–341` — wall-clock timeout остаётся; `G/report/report.go:201–204` — убирается только `time_ms`. | и то и другое |
| 17 | Неверно описаны counters и `temporal.source` | ПОДТВЕРЖДЕНО | `S/references/engine-tools.md:207,209` — counters «same for every property», source `claim`; `G/report/report.go:208–214` подставляет отдельные temporal stats, `:140` перечисляет `never-claim` и `accept-labels`. | текст skill |
| 18 | Устаревшие experimental LTL и CLI estimate | ПОДТВЕРЖДЕНО | `S/assets/report-template.md:73–74` — «experimental LTL evidence»; `G/report/report.go:251–252` требует `exhaustive` для `verified`. `S/references/engine-tools.md:180` предлагает грубую замену estimate; `G/cli/cli.go:280–282` уже имеет `--estimate`. | текст skill |
| 19 | Активация по любому слову конфликтует с исключениями | ЧАСТИЧНО | `S/SKILL.md:14–22` — «use … for any» включает «гонка», но затем исключает data-race linters. При этом `:7–9` задаёт более узкое намерение — проверку поведения взаимодействующих частей. Противоречие формулировок есть; фактические ложные срабатывания без проверки активации не установлены. | текст skill |

## Находки A–D, не выделенные отдельным пунктом §E

| № | Краткая суть | Вердикт | Чем подтверждается/опровергается | Где правка |
|---|---|---|---|---|
| A1 | Буквальные FR-009/010 шире принятого scope | ПОДТВЕРЖДЕНО | `N/11-skill-requirements.md:346–347` требует SPIN и NuSMV/BDD/BMC workflows; `N/14-skill-building-plan.md:275–278` явно предлагает изменить эти требования. Это расхождение документов, а не самовольное ограничение skill. | текст skill |
| A2 | Обещанный сравнительный eval A7 отсутствует | ПОДТВЕРЖДЕНО | `N/14-skill-building-plan.md:305` — «Отдельный eval … словесное описание → IR → mc_check»; `S/evals/evals.json:6,73,147,228,302,362,422` перечисляет все семь текущих заданий: такого сравнения среди них нет. `S/SKILL.md:81–82` честно называет direct IR experimental. | evals |
| B6 | Для всех temporal нарушений обещано лассо, для всех safety — его отсутствие | ПОДТВЕРЖДЕНО | `S/SKILL.md:175–176` — «A temporal violation is a lasso»; `S/references/counterexamples.md:28` — safety «no loop at all». `F/g4-ltl.feature:81–87` фиксирует нарушение never claim без loop; `C/g4-confirmation.md:95` различает формы собственного автомата и claim. | текст skill |
| B8 | Таблица `bounded` включает time/memory, хотя ниже их исключает | ПОДТВЕРЖДЕНО | `S/references/evidence-and-status.md:82` — «time or memory limit» в bounded; `:91–99` — time/memory дают unknown; `G/explore/explore.go:238–244` возвращает bounded только для state/depth/process stop. | текст skill |
| B9 | Заполненный канал назван overflow | ПОДТВЕРЖДЕНО | `S/references/promela-subset.md:61` — «channel capacity exceeded» в `invalid-model`; `G/explore/explore.go:818` — отправка при полной ёмкости просто не разрешена: `ChanLen < Capacity`. | текст skill |
| B11 | Lint обещан для каждого свойства и как проверка достижимости атомов | ПОДТВЕРЖДЕНО | `S/SKILL.md:114` — «on every property»; `S/references/properties-ltl-ctl.md:156` — «constant on all reachable states»; `G/mcp/lint.go:73–74` отвергает progress, `:285–286` лишь советует отдельный `reach`. | текст skill |
| B13 | Совпадение счётчиков во всех 43 LTL-тройках преувеличено | ПОДТВЕРЖДЕНО | `S/references/evidence-and-status.md:103–106` — «verdicts and product state counts»; `C/g4-confirmation.md:95` — под `-f` и собственным автоматом счётчики с `pan` «не сравниваются». | текст skill |
| B14 | `stutter_invariant` обещан в каждой temporal записи | ПОДТВЕРЖДЕНО | `S/references/properties-ltl-ctl.md:65–66` — «Every temporal property record carries it»; `G/report/report.go:149` — поле `omitempty`; `G/explore/cycle.go:313–315` заполняет его в ветке скомпилированной LTL-формулы. | текст skill |
| B16a | Класс размера и метрика памяти описаны неточно | ПОДТВЕРЖДЕНО | `S/references/evidence-and-status.md:135` — для small вектор «—»; `G/estimate/estimate.go:243–247` требует ≤128 байт и для small. `S/assets/report-template.md:50` просит peak memory; `G/report/report.go:174–176` содержит `memory_bytes_est`. | текст skill |
| B16b | Остались устаревшие примеры never claim и `provided` | ПОДТВЕРЖДЕНО | `S/SKILL.md:103–104,249–251` — ожидание «never claim not executed» и «until then» для `provided`; `G/frontend/promela/lower.go:473–476` создаёт свойство `never`; `S/references/promela-subset.md:123–127` относит `provided` к subset. | текст skill |
| C1 | Inline IR теряет `#define` для формул | ПОДТВЕРЖДЕНО | `S/SKILL.md:155` — «Pass the IR»; `G/mcp/check.go:242–246` — при `in.IR != nil` выполняется `defines = nil`. Нужен session с исходным Promela либо заранее раскрытая формула. | текст skill |
| C2 | Узел fairness не описывает обычные исходы без лассо | ПОДТВЕРЖДЕНО | `S/references/workflow.md:89–93` после первого запуска предлагает только ветви с лассо; `:15–25` обещает исчерпывающую развилку каждого узла. `verified`, `inconclusive` и rejection там не разобраны. | текст skill |
| C3 | «Каждый mc_* имеет CLI equivalent» чрезмерно | ПОДТВЕРЖДЕНО | `S/SKILL.md:91–95` — «every `mc_*` … CLI equivalent»; `S/references/engine-tools.md:174–180` признаёт отсутствие CLI simulation и ручной lint, при этом `G/cli/cli.go:280` уже реализует estimate. | текст skill |
| D1 | E3 ложно проваливает законный parse→IR→check | ПОДТВЕРЖДЕНО | `S/evals/evals.json:163–166` требует строку `mcd check … --petri`; `S/references/engine-tools.md:164–170` описывает добавление свойств в IR и `mcd check --ir`; `C/g6-confirmation.md:380–383` фиксирует именно этот случай. | evals |
| D2 | E4 обучает устаревшему обходу Busy через progress | ПОДТВЕРЖДЕНО | `S/evals/evals.json:231` — «control-label atoms are not accepted … cannot … directly»; `F/g5-ctl-v1.feature:105–114` проверяет `AG EF subscriber@Idle` непосредственно. | evals |
| D3 | E6 принимает любой статус, хотя ожидается конкретный CTL-результат | ПОДТВЕРЖДЕНО | `S/evals/evals.json:413–416` — regex всех шести статусов; `F/g5-ctl-v1.feature:105–113` — для данного случая `verified`/`exhaustive` и отсутствие run. | evals |

## 2. Три критичные находки

**DFS и `bounded`.** Логика первого рецензента верна: состояние может впервые встретиться по длинному пути, сохраниться за пределом D и не раскрыться; при последующем коротком пути `if !isNew { continue }` не даст раскрыть его повторно (`G/explore/explore.go:1501–1522`). Тогда остаются непроверенные поведения длиной **не больше D**. Более того, движок вызывает `checkState` **до** проверки глубины (`:1509–1522`), поэтому неверна и вторая половина обещания «nothing beyond»: свойство может решиться на состоянии D+1. Это **дефект текста и контракта отчёта движка**: `budgetEvidence` выдаёт `Bounded` (`:238–244`), а документация определяет его как полное покрытие до названной границы (`S/references/evidence-and-status.md:54,81–82`). Пока такое определение действует, при данном DFS-stop `bounded` выдаваться не должен. Правильный выход — обеспечить реальное покрытие горизонта (например, BFS либо пересмотр состояния при более коротком пути) и подтвердить тестом; иначе выдавать `unknown` с числом просмотренных состояний как метрикой. Сохранить токен `bounded`, просто ослабив его значение до «обход остановился при лимите», технически можно, но это уже изменение публичного смысла evidence, которое надо провести через требования, features и отчёты.

**Регрессия `reach`.** Ошибка текста прямая: `reach bad` отвечает на вопрос, найдено ли bad; при полном поиске без bad движок возвращает `violated` (`G/explore/explore.go:1763–1767`). Для отдельного свойства `invariant !bad` либо LTL `[] !bad` ожидается `verified` при исчерпывающем поиске. Первый рецензент слегка сократил условие: после исправления `reach bad` станет `violated` **только при завершённом поиске**; при исчерпании бюджета возможен `inconclusive` (`:1773–1776`). Формулировка регрессии должна назвать свойство, статус и полноту каждого запуска отдельно.

**`mc_explain` и replay.** Критика верна: `mc_explain` читает сериализованную трассу и форматирует prefix/loop (`G/mcp/check.go:460–489`), но не исполняет шаги заново. Guided `mc_simulate` принимает последовательность `edges` (`G/mcp/simulate.go:24–31,87–88`), а не ID трассы. «Трасса декодирована через `mc_explain`» — точное утверждение. «Трасса воспроизведена» допустимо лишь после отдельного guided запуска с извлечёнными шагами и сверки остановки, конечного состояния и, для лассо, повторяемого цикла. Сам вызов `mc_explain` этого не доказывает.

## 3. Что первый рецензент не заметил

- Граница глубины нарушена **в обе стороны**: не только пропускаются состояния в горизонте D, но и проверяется новое состояние глубины D+1 до отсечения (`G/explore/explore.go:1509–1522`; аналогично BFS `:1639–1654`). Поэтому фраза `S/references/evidence-and-status.md:82` — «nothing beyond» — также неверна.
- Приёмочный feature закрепляет неисполняемые Petri-примеры: `F/g3-skill-package.feature:115–122` требует буквальные `AG EF fire` и `[]<> fire`. Замена примеров только в reference сломает текстовую приёмку; сам feature нужно обновить под `enabled(t)` с оговоркой о LTL-прокси.
- В дереве цель «bounded assurance» заранее объявлена evidence `bounded` или статусом `inconclusive` (`S/references/workflow.md:51–55`). Найденный на таком запуске контрпример даст `violated`/`exhaustive` (`G/explore/explore.go:1302–1308`), а найденная цель `reach` — `verified`/`exhaustive` (`:1311–1320`). Это самостоятельная ошибка узла 1.
- E6 требует объяснить, является ли свидетель «path or tree» (`S/evals/evals.json:405–409`), тогда как именно его `AG EF subscriber@Idle` возвращает **без run** с объяснением отсутствия (`F/g5-ctl-v1.feature:105–113`). Критерий оценивания принуждает обсуждать форму несуществующей трассы.

Ошибочной нумерации проверенных ссылок первого рецензента на исходники я не нашёл. Разногласия ниже относятся к выводам из строк, а не к отсутствующим цитатам.

## 4. Где первый рецензент преувеличил

- Предложение для №1 «токен `bounded` можно сохранить, поправив текст» недостаточно при текущем контракте. `S/references/evidence-and-status.md:54` определяет его как покрытие всей области до границы, а `G/explore/explore.go:238–244,1505–1522` этого для DFS по глубине не гарантирует. Нужно менять алгоритм или выдачу evidence, либо явно принимать новое, более слабое определение во всех слоях.
- Для №9 не всякий ручной вывод является подменой статуса. `S/references/fairness.md:130` прямо говорит «status field stays the engine's `verified`» при логическом следствии о strong fairness. Анализ конкретного сильно справедливого лассо также может обосновать фразу о контрпримере; ошибка в `:143–144` — запись этого вывода словом `violated` без чёткого отделения от `not-executed` движка (`G/explore/cycle.go:285–288`).
- Для №15 статически установлено, что конфигурация выбирает POSIX-скрипт (`model-check-plugin/mcp/servers.json:5`; `G/bin/mcd:1`), и что Windows не запускали (`C/g6-confirmation.md:35–37,509–510`). Это достаточная причина ограничить заявление о валидации, но недостаточная для вердикта о фактическом отказе Windows-клиента без запуска.
- Замечание о том, что `mc_estimate` не предупредит заранее, требует уточнения. Workflow действительно вызывает estimate **до** check (`S/SKILL.md:144–149`), а estimate классифицирует размер по числу состояний и ширине вектора (`G/estimate/estimate.go:243–255`). Проблема точнее: он не прогнозирует память на состояние (`C/g5-confirmation.md:417–418`), поэтому предупреждение не гарантирует укладывания в 60 с/1 ГБ.

## 5. Порядок исправления и проверка

| Пакет | Что закрыть одним проходом | Чем проверить |
|---|---|---|
| 1. Семантика результата | №1–3, 7, 16; дополнительные B6, B8 и ошибка узла 1. Согласовать `reach`, форму трассы, decode/replay и смысл `bounded`; изменить DFS/evidence для глубины. | Новый минимальный «ромб» графа, где одно состояние впервые найдено глубже D, затем короче; проверить DFS и BFS, отсутствие ложного покрытия и свидетеля D+1. Дополнить `F/g0-engine.feature`, `F/g2-mcp.feature`, `F/g4-ltl.feature`; отдельный regression probe `reach bad` до/после исправления модели. |
| 2. Свойства и возможности | №5–6, 10–13, 18; дополнительные B9, B11, B14, B16b. Обновить CTL, Petri `enabled(t)`, lint, отказы входа и реально доступные поля JSON. | `F/g5-ctl-v1.feature` для `AG EF`, `E[p U q]`, `P:pid@label` и отсутствующего witness; Petri probe через parse→IR→check; `F/g3-skill-package.feature` после замены требований к `fire`; `F/g4-ltl.feature` для конечной трассы и never claim. |
| 3. Сохранность запросов | №8, 14; C1–C3. Перед `mc_check` объединять свойства frontend и пользовательские, сохранять точный запрос/формулы и различать session IR от inline IR с макросами. | MCP probe: модель с `deadlock`/`never`/`progress` плюс пользовательский `reach`, затем проверить все записи отчёта и manifest; отдельный probe с `#define` и формулой через session и inline IR; сценарии `F/g2-mcp.feature`. |
| 4. Границы доказательств и поставки | №9, 15, 17, 19; дополнительные A1, A2, B13, B16a. Уточнить strong fairness, метрики, платформенную матрицу, триггеры и scope. | `F/g4-ltl.feature` для strong fairness; сверка JSON отчёта с `G/report/report.go`; `F/g6-package.feature` и реальные smoke-прогоны каждого заявленного launcher; отдельный A7 eval из `N/14-skill-building-plan.md:305`. |
| 5. Оценивание агента | №4 и дополнительные D1–D3. Привязать assertions к журналу фактических вызовов и JSON-отчётам; принять оба законных маршрута E3; исправить E4 и E6. | Прогон grader’а на заведомо выдуманном ответе **без** вызовов должен провалиться; затем полный прогон с артефактами `mc_check`. Для E6 требовать `verified`/`exhaustive`, `temporal.logic=ctl` и объяснение отсутствия run; для E3 принять `--petri` и parse→`--ir`. |

**Одним текстом skill не закрыть:** №1 — если сохраняется нынешний смысл `bounded`, требуется движок; №4, A2, D1–D3 — evals/grader; №12 — также приёмочный feature; №14 — движок, если manifest должен сам хранить точные property expressions и сопровождающие артефакты. Для №15 код launcher и платформенный запуск нужны, только если заявлять нативную поддержку Windows; честное ограничение текущей валидации исправляется текстом.
