# Согласование по реализации — Codex (gpt-6-astra)

Дата: 2026-09-29. Инструмент: `codex exec --json -s read-only --json resume <thread>`,
модель `gpt-6-astra`, тот же тред, что у [review-astra-plan.md](review-astra-plan.md) и
[review-astra-skill.md](review-astra-skill.md): рецензент оценивает исполнение
**собственных** находок.

Предмет: коммиты `897ca34..HEAD` и рабочее дерево (рецензент сам отделил незакоммиченные
изменения), против [review-fixes-confirmation.md](review-fixes-confirmation.md),
[review-fixes-logika.md](review-fixes-logika.md) и
`features/r1-review-fixes.feature`. Задание требовало статуса по каждой находке
(ЗАКРЫТО / ЧАСТИЧНО / НЕ ЗАКРЫТО / **ЗАКРЫТО НЕВЕРНО**), отдельного раздела про правки,
которые сами неверны, и оценки решения по `bounded` и по evals.

Итог рецензента: 7 ЗАКРЫТО, 8 ЗАКРЫТО ЧАСТИЧНО, 4 ЗАКРЫТО НЕВЕРНО из 19 находок §E;
**согласования полного закрытия он не даёт**. Найдено четыре ошибки в самих правках,
включая воспроизведённый обход новой проверки evals (раздел 4).

Что сделано по этому отчёту — в §7 подтверждения
[review-fixes-confirmation.md](review-fixes-confirmation.md).

Ниже — ответ рецензента дословно; пути в ссылках приведены к путям от корня репозитория.

---

## 1. Таблица закрытия

«ЗАКРЫТО» ниже означает закрытие конкретной находки, а не готовность всей возможности.

| № | Суть | Статус | Чем именно: новый текст | Что осталось |
|---|---|---|---|---|
| E1 | `bounded` обещал полный горизонт | **ЗАКРЫТО ЧАСТИЧНО** | [evidence-and-status.md:103][es]: «does not say that everything below the bound was searched» | [engine-tools.md:251][et] всё ещё: «the result holds up to it». Подробнее §3. |
| E2 | Перевёрнутая регрессия `reach bad` | **ЗАКРЫТО** | [counterexamples.md:197][cex]: «`reach bad` … **`violated`**»; :204 отдельно вводит invariant `!bad` | Сам рецепт исправлен. Новое необоснованное «good news» в SKILL разобрано ниже. |
| E3 | Decode выдавался за replay | **ЗАКРЫТО НЕВЕРНО** | [counterexamples.md:38][cex]: «decodes»; :182 добавлен guided-рецепт | Различение правильное, но новый рецепт не гарантирует воспроизведения той же трассы и не работает для произвольного лассо. |
| E4 | Evals принимали рассказ за запуск | **ЗАКРЫТО НЕВЕРНО** | [grader.py:192][grader]: «The engine actually ran»; добавлен `engine_report` | Проверяется похожий JSON, без происхождения и связи с требуемой проверкой. E8 обходится на 8/8. |
| E5 | Запрещён работающий CTL | **ЗАКРЫТО ЧАСТИЧНО** | [SKILL.md:142][skill]: «`ctl` is executed since G5» | [fixtures/README.md:35][fixtures]: «E6 … is still `not-executed`». |
| E6 | Обещано дерево CTL | **ЗАКРЫТО ЧАСТИЧНО** | [counterexamples.md:32][cex]: «the engine builds no tree» | [properties-ltl-ctl.md:146][props] сохраняет «a tree for violations»; новая ссылка на `temporal.note` вместо `witness_note` неверна. |
| E7 | Explain обязателен для любого `violated` | **ЗАКРЫТО ЧАСТИЧНО** | [SKILL.md:195][skill]: «that carries a `counterexample`» | [engine-tools.md:158][et] по-прежнему «every `violated`»; [report-template.md:66][tpl] требует хронологию «Per `violated` property». Для недостижимого `reach` обещано отсутствующее поле. |
| E8 | Пользовательский список теряет автоматические свойства | **ЗАКРЫТО ЧАСТИЧНО** | [SKILL.md:173][skill]: «replaces the model’s own»; :176 — вернуть свойства либо сохранить оба отчёта | Основной маршрут исправлен. [petri-nets.md:176][pn] всё ещё обещает `deadlock` и `safe` «in every report». |
| E9 | Ручной strong fairness менял engine status | **ЗАКРЫТО** | [fairness.md:148][fair]: «status does not change»; :161 — bounded counter «is **not** an encoding» | Закрыта именно подмена статуса и эквивалентности. Полнота ручного перечисления явно оставлена ответственностью агента. |
| E10 | Rejection смешан с `invalid-model` | **ЗАКРЫТО ЧАСТИЧНО** | [promela-subset.md:136][ps]: «status is still `not-executed`»; [SKILL.md:236][skill] различает случаи | [SKILL.md:187][skill] сохраняет «channel capacity» как overflow; новая формулировка «never in `invalid-model`» забывает `d_step`. |
| E11 | Неработающие примеры CTL | **ЗАКРЫТО НЕВЕРНО** | [properties-ltl-ctl.md:48][props]: `proc:pid@label`; :78 — `A[p U q]` | Добавлено ложное «1-based pid»; в таблице :113 осталось `A(p U q)`. |
| E12 | Несуществующий `fire(t)` и отсутствующий рецепт | **ЗАКРЫТО ЧАСТИЧНО** | [petri-nets.md:147][pn]: «There is no `fire(t)` atom»; :158 различает точную CTL-замену и LTL-прокси | Новый совет считать бесконечные срабатывания накапливающим местом приводит к overflow. В :132 также осталось «atom in G4». |
| E13 | Lint якобы проверяет полярность claim | **ЗАКРЫТО ЧАСТИЧНО** | [properties-ltl-ctl.md:164][props]: «no polarity note» | [pitfalls.md:23][pit] и [counterexamples.md:144][cex] всё ещё предлагают `mc_lint_property` polarity note. |
| E14 | Manifest якобы хранит карточку, mapping и выражения | **ЗАКРЫТО** | [workflow.md:110][wf]: «takes **only** `session_id`»; :118 — «save the `mc_check` request itself» | Инструкция теперь требует отдельные артефакты. Сам manifest по-прежнему их не хранит — это больше не скрыто. |
| E15 | Необоснованная платформенная валидация | **ЗАКРЫТО** | [engine-tools.md:315][et]: «validated on linux/arm64, built for the rest» | Закрыто ложное обещание. Работоспособность остальных платформ этим не установлена. |
| E16 | Побайтная воспроизводимость при timeout | **ЗАКРЫТО** | [engine-tools.md:277][et]: «runs that were not stopped by the clock» | Существенное исключение теперь соответствует коду. Краткую строку флага :67 желательно отослать к этой оговорке. |
| E17 | Неверные counters и `temporal.source` | **ЗАКРЫТО** | [engine-tools.md:206][et]: «its **own** product or labelling»; :208 — `never-claim`, `accept-labels` | Эти два поля исправлены. Другие ошибки temporal-схемы перечислены отдельно. |
| E18 | Experimental LTL и грубая замена estimate | **ЗАКРЫТО** | [workflow.md:134][wf]: «exception ended»; [engine-tools.md:180][et]: `mcd check --estimate` | Эти устаревшие ограничения устранены. |
| E19 | Активация по любому слову | **ЗАКРЫТО** | [SKILL.md:21][skill]: «What decides is the question, not the word» | Текстовый конфликт устранён; качество новых триггеров в рассматриваемых коммитах не измерено. |

Находки A–D, повторяющие E, учтены выше. Ниже — остальные; обозначения A1–D3 соответствуют таблице второго рецензента.

| № | Суть | Статус | Чем именно: новый текст либо отсутствие правки | Что осталось |
|---|---|---|---|---|
| A1 | FR-009/010 шире принятого scope | **НЕ ЗАКРЫТО** | [11-skill-requirements.md:346][req]: «SPIN/Promela workflow»; :347 — «BDD/BMC» | Нормативный scope не согласован. |
| A2 | Сравнительная проверка A7 | **ЗАКРЫТО ЧАСТИЧНО** | [evals.json:538][eval]: добавлен `ir-from-words` | Проверяет преимущественно Promela-маршрут, не сравнивает модели; assertions допускают отсутствие проверки mutex. |
| B6 | Все temporal-трассы — лассо; safety — без loop | **ЗАКРЫТО ЧАСТИЧНО** | [counterexamples.md:28][cex]: «Read the shape from the record» | [engine-tools.md:244][et]: temporal `violated` — «an acceptance cycle»; [properties-ltl-ctl.md:145][props]: «LTL violation: one lasso». |
| B8 | Time/memory ошибочно относились к `bounded` | **ЗАКРЫТО** | [evidence-and-status.md:84][es]: time/memory → `unknown` | Классификация исправлена; объяснение через «можно назвать границу» остаётся неудачным, §3. |
| B9 | Полный канал назван overflow | **ЗАКРЫТО НЕВЕРНО** | [promela-subset.md:61][ps]: «never in `invalid-model`» | Старое обобщение заменено другим неверным обобщением; исключение `d_step` реально исполняется. |
| B11 | Lint для всех kinds и поиск достижимости внутри lint | **ЗАКРЫТО ЧАСТИЧНО** | [SKILL.md:125][skill] перечисляет четыре kind; [properties-ltl-ctl.md:163][props]: «syntactic only» | В таблице инструментов осталось «every property before a check» — [engine-tools.md:160][et]. |
| B13 | Совпадение счётчиков всех 43 троек | **ЗАКРЫТО ЧАСТИЧНО** | [evidence-and-status.md:121][es]: «where they are comparable» | [properties-ltl-ctl.md:207][props] сохраняет безусловное «product state counts do agree». |
| B14 | Обязательный `stutter_invariant` | **ЗАКРЫТО НЕВЕРНО** | [properties-ltl-ctl.md:67][props]: «`ltl` and `progress`» | Для `progress` поле не заполняется. Таблица [engine-tools.md:208][et] также обещает отсутствующие обязательные поля. |
| B16a | A4, ширина вектора и RSS | **ЗАКРЫТО** | [evidence-and-status.md:152][es]: small ≤128 bytes; :163 — «target, not a guarantee»; [report-template.md:50][tpl]: `memory_bytes_est` | Исправлено описание границ, а не доказана производительность всех temporal-моделей. |
| B16b | Устаревшие never claim / `provided` | **ЗАКРЫТО ЧАСТИЧНО** | [SKILL.md:114][skill]: executable `never`; :276 — `provided` поддержан | [engine-tools.md:80][et] всё ещё предлагает предупреждение «never claim … not executed». |
| C1 | Inline IR теряет макросы | **ЗАКРЫТО** | [SKILL.md:169][skill]: использовать `session_id` либо раскрыть макросы | Соответствует `defines = nil` при inline IR — [mcp/check.go:245][check]. |
| C2 | Fairness-узел описывает только лассо | **ЗАКРЫТО ЧАСТИЧНО** | [workflow.md:89][wf]: добавлены `verified` и budget stop | Объявлены «Three outcomes», но отсутствуют `invalid-model`, rejection и `not-executed`. |
| C3 | Любой MCP-инструмент имеет CLI-аналог | **ЗАКРЫТО** | [SKILL.md:96][skill]: «most»; :98–100 перечисляют simulation/lint/estimate | Деградации теперь названы в основном маршруте. |
| D1 | E3 отвергает parse→IR→check | **ЗАКРЫТО ЧАСТИЧНО** | [evals.json:169][eval] принимает оба CLI-маршрута | По-прежнему не принимает штатный MCP-маршрут. |
| D2 | E4 принуждает к progress вместо CTL | **ЗАКРЫТО НЕВЕРНО** | [evals.json:243][eval]: «Two legitimate routes» | Assertions :256 и :298 всё ещё требуют LTL/progress и изменения модели; новый CTL-рецепт также ослабляет вопрос. |
| D3 | E6 принимает любой статус | **ЗАКРЫТО ЧАСТИЧНО** | [evals.json:431][eval]: конкретные `verified/exhaustive` | `engine_report` не требует CTL или нужной формулы; фактически зачтён `deadlock`. |

Дополнительные замечания моего A–D, не выделенные в таблице второго рецензента:

| № | Суть | Статус | Основание | Что осталось |
|---|---|---|---|---|
| A/E2 | План ожидает зависимость alternating bit от fairness | **НЕ ЗАКРЫТО** | [14-skill-building-plan.md:230][plan]: «статусом, зависящим от fairness» | E2 исправлен, нормативная таблица нет; E2b в неё не внесён. |
| B3 | `verified` для reach не требует полного обхода | **ЗАКРЫТО** | [SKILL.md:231][skill]: «except for `reach`» | Отдельная новая оценочная ошибка «good news» остаётся, §2. |
| B14/formula | LTL без formula ошибочно запрещён | **НЕ ЗАКРЫТО** | [engine-tools.md:157][et]: formula «required» | [mcp/check.go:39][check]: «omitted = the model’s own never claim». |
| C/триггеры | Нет измерения новой description | **НЕ ЗАКРЫТО** | [g6-package.feature:131][g6feature]: `@pending` | Незакоммиченные новые измерения не входят в этот вердикт. |
| C/дублирование | Контракт повторён и расходится | **НЕ ЗАКРЫТО** | [engine-tools.md:251][et]: «holds up to it» против [evidence-and-status.md:104][es]: «does not say … below the bound» | Противоречия остались, вопреки подтверждению. |
| C/раскрытие | Обязательная справка вне skill | **НЕ ЗАКРЫТО** | [properties-ltl-ctl.md:88][props]: authority — `steps`/`features`; «has not yet had its row-by-row pass» | Самодостаточный контракт CTL не собран. |
| D/узнавание | Известные ответы подменяют проверку формализации | **НЕ ЗАКРЫТО** | [handshake.pml:3][handshake]: сравнение «same questions, same verdicts»; [evals.json:551][eval] проверяет слово `proctype` | Ни соответствия модели словам, ни различения корректных и вырожденных моделей. |
| D/покрытие | Нет негативных и сквозных evals | **ЗАКРЫТО ЧАСТИЧНО** | [evals.json:424][eval] добавлено отсутствие run; :538 — E8 | Нет проверок сохранности свойств, replay, разных budget stops, strong/CTL fairness rejection и полного установленного MCP-маршрута. |

## 2. Правки, которые сами неверны

**1. `engine_report` назван доказательством исполнения, хотя проверяет узнаваемый объект.**

Новый текст: «needs the JSON the engine itself produced». [grader.py:196][grader]

Код проверяет `engine.name == "mcd"`, префикс `report_schema` и наличие массива `properties`; затем сравнивает только `property`, `status`, `evidence`. [grader.py:175][grader], [grader.py:199][grader]

**Вывод:** происхождение отчёта, модель, формула, fairness и связь с вызовом не проверяются. Ошибка не гипотетическая: в переградации CTL-проверка зачтена по `deadlock`. Подробности и воспроизводимый обход — §4.

**2. Новый рецепт replay может воспроизвести другую трассу или отвергнуть правильную.**

Новый текст: передать каждый `command`/`user_name` как `edges`; «`edge not enabled` means the list and the model disagree»; лассо повторить нужное число раз. [counterexamples.md:182][cex]

Код прямо предупреждает: имена не уникальны, при совпадении симулятор «takes the first in the explorer’s order». Уникален только `process/index`. [mcp/simulate.go:52][simulate]

Кроме того:

- симуляция строит `NewStepper(m)` исходной модели, а автомат LTL добавляется отдельно при проверке: `mm.Processes = append(..., c.Process)`. Шаги этого автомата из decoded-трассы не являются рёбрами исходной модели. [mcp/simulate.go:118][simulate], [explore/cycle.go:310][cycle]
- неизвестное имя отвергается **до запуска**: «no edge named». [mcp/simulate.go:124][simulate]
- успешное достижение тупика или failing assert возвращает `deadlock`/`assert failed`, а не обязательно `edges exhausted`. [mcp/simulate.go:148][simulate], [mcp/simulate.go:191][simulate]

**Вывод:** нельзя считать неудачу такого рецепта дефектом модели, а успех — проверкой исходного counterexample. Нужны однозначное сопоставление рёбер и участников rendezvous, проекция служебных шагов, сверка состояний и замыкания цикла. Пока этого нет, следует обещать только ограниченную ручную проверку выбранной последовательности. FR-015 не становится выполненным от замены слова *replayed* на *decoded*.

**3. Исправление CTL-синтаксиса добавило ошибочную нумерацию PID.**

Новый текст: «the engine’s own **1-based pid**». [properties-ltl-ctl.md:50][props]

Код разрешает точное имя экземпляра: `Process(...).Name == name`; имена строятся как `"%s:%d"` из PID. [ctl/ctl.go:184][ctlparse], [frontend/promela/lower.go:380][lower]

Проверил бинарником модель с двумя `P`: `P:0@Idle` и `P:1@Idle` принимаются; `P:2@Idle` отвергается: «no process called P:2».

**Вывод:** следует брать имя экземпляра из IR, а не прибавлять единицу. Дополнительно в таблице осталось неисполняемое `A(p U q)` — [properties-ltl-ctl.md:113][props]; парсер распознаёт until только после `[` — [ctl/ctl.go:483][ctlparse].

**4. В примере Busy незаметно ослаблено требование.**

Новый текст: «Can it get stuck in Busy?» объявлен branching-вопросом и непосредственно заменён на `AG EF switch@Idle`; далее говорится о «same question in LTL». [SKILL.md:280][skill]

Код различает эти кванторы: `EF p` нормализуется в `E[true U p]`, а `AF p` — в `!EG !p`. [ctl/normal.go:52][ctlnormal]

**Вывод:** возможность когда-нибудь вернуться в Idle не исключает бесконечный путь без возврата. Проверил небольшую модель с выбором «остаться в Busy / вернуться»: `AG EF Idle` — `verified`, `AG(Busy -> AF Idle)` — `violated`.

На самом `CH14/version1` **обе** формулы дали `verified`, 9 состояний. Поэтому результат примера численно верен, но инструкция учит подменять вопрос. Следует сначала различить «возврат всегда возможен» и «возврат неизбежен»; для второй трактовки проверять соответствующий `AF`, сохраняя выбранное пользователем условие.

**5. Обработка отсутствующего свидетельства направляет к несуществующим полям.**

Новый текст включает недостижимый `reach` в перечень результатов, которые «carry `temporal.witness_note` instead». [SKILL.md:197][skill], [counterexamples.md:34][cex]

Код для недостижимого reach устанавливает только статус, evidence и `Reason`: «no reachable state satisfies …». [explore/explore.go:1765][explore]

В соседней новой строке объяснение вложенного CTL отнесено к `temporal.note`. [counterexamples.md:32][cex] Код записывает его в **`WitnessNote`**: `info.WitnessNote = w.Note`. [explore/ctlcheck.go:130][ctlcheck]

**Вывод:** нужны разные маршруты: `reach.reason`; CTL `temporal.witness_note`; общий `temporal.note` — отдельная семантическая справка. Совет повторно проверить подформулу «at that state» тоже не является готовым вызовом: CTL оценивает `sat[0]`, а вход `mc_check` не принимает стартовое состояние. [explore/ctlcheck.go:109][ctlcheck], [mcp/check.go:44][check]

Отдельно новое «`violated`, and that is the good news» опять безусловно оценивает недостижимость. [SKILL.md:198][skill] Для sanity reach недостижимый trigger — плохая новость. Логическое ревью уже исправляло именно это обобщение, но только в другом файле. [review-fixes-logika.md:20][logic]

**6. `stutter_invariant` ошибочно обещан для progress.**

Новый текст: поле несут записи со скомпилированной формулой — «`ltl` and `progress`». [properties-ltl-ctl.md:67][props]

Код ветки `KindProgress` создаёт `npClaim`, но не устанавливает поле; `info.StutterInvariant = &si` находится только в ветке `prop.Formula != ""`. [explore/cycle.go:293][cycle], [explore/cycle.go:313][cycle]

Проверил progress-модель: temporal содержит `logic`, `source: np`, `fairness`, `claim`, без `stutter_invariant` и `formula`.

**Вывод:** поле определяется источником свойства и наличием скомпилированной формулы, а не просто kind. «Always … formula» в [engine-tools.md:208][et] также неверно.

**7. Уточнение про полный канал стало слишком сильным.**

Новый текст: отправка в полный канал «can only end in a deadlock, never in `invalid-model`». [promela-subset.md:61][ps]

Обычная отправка действительно блокируется через `ChanLen < Capacity`. [explore/explore.go:818][explore] Но при блокировке продолжения `d_step` код возвращает «block in d_step seq». [explore/explore.go:1097][explore]

Проверил `d_step { q!1; q!2 }` при ёмкости 1: результат `invalid-model`.

**Вывод:** правильно писать об обычной блокировке **вне продолжения `d_step`**. Нынешний текст противоречит и коду, и следующей строке собственной таблицы.

**8. Petri-рецепт наблюдения бесконечных срабатываний меняет результат на overflow.**

Новый текст предлагает «add a counter place that the firing of `t` increments». [petri-nets.md:164][pn]

Код переводит каждое место в `ir.Byte` и добавляет токены обычным сложением. [frontend/petri/petri.go:213][petri], [frontend/petri/petri.go:237][petri]

**Вывод:** накапливающий счётчик не может наблюдать бесконечное число срабатываний в этом движке: он переполнится. Проверка такого места с capacity 2 вернула `invalid-model` на третьем срабатывании, в том числе для LTL. Нужен конечный наблюдатель событий с определённой семантикой сброса либо честный отказ от точной проверки firing; предложенный счётчик следует убрать.

## 3. Решение по `bounded`

**Ослабление допустимо как явно согласованное изменение контракта, но нынешней правки недостаточно.**

Новый подробный абзац точно описывает DFS: сохранённое за пределом D состояние не раскрывается при повторном достижении коротким путём; свойства проверяются до проверки глубины. Это соответствует `if !isNew { continue }`, затем `checkState`, затем depth check. [evidence-and-status.md:103][es], [explore/explore.go:1505][explore]

Но прежний сильный смысл остаётся:

- прямое «the result holds up to it» — [engine-tools.md:251][et];
- обещание «bounded assurance» через объявленный states/depth budget — [workflow.md:54][wf];
- разрешённая формулировка «inconclusive **beyond** that bound» не сообщает о непроверенных путях **внутри** глубины — [evidence-and-status.md:196][es].

Есть и проблема самого нового определения. «Можно назвать границу остановки» не отличает `bounded` от `unknown`: лимиты времени и памяти тоже заданы числами. Код содержит явный `TimeMS` и сообщение «memory budget … exceeds …». [cli/cli.go:338][cli], [explore/explore.go:1364][explore] Фактическое правило — перечень типов остановки, а не общая именуемость границы.

Я бы согласился оставить токен для совместимости только при трёх условиях: он везде определяется как **неполный обход, остановленный лимитом states/depth/processes**; из него нигде не выводится покрытие горизонта; требования, examples и отчёты используют тот же смысл.

**Альтернатива требует изменения движка, но не обязательно алгоритма DFS.** Сейчас `budgetEvidence` явно возвращает `Bounded` для `"depth budget"`. Чтобы выдавать `unknown`, нужно изменить эту политику и соответствующие тесты. [explore/explore.go:238][explore]

План 14 тоже требует изменения: он прямо предписывает depth stop → `bounded`. [14-skill-building-plan.md:163][plan] Однако ослаблять требования 11 для такой альтернативы необязательно: их исходный пример bounded — отсутствие контрпримеров длины до `k`, то есть как раз более сильный смысл. [11-skill-requirements.md:153][req] Переписывать DFS потребуется, если решено **обеспечить** полный горизонт, а не просто перестать его подразумевать.

## 4. Evals

**Улучшение есть:** одного текста «я запускал mcd» теперь недостаточно. Без JSON девять проверок `engine_report` провалятся. Но набор по-прежнему не отличает выполненную нужную проверку от сочинённого или неподходящего артефакта. [grader.py:201][grader]

Я передал grader через `StringIO`, без создания файлов, следующий объект:

```json
{
  "engine": {"name": "mcd", "report_schema": "mcd-report/not-a-version"},
  "properties": [
    {"id": "deadlock", "status": "verified", "evidence": "exhaustive"}
  ]
}
```

И текст:

> proctype Promela first assert. Модель соответствует описанию. rendezvous.

**Eval 8 получил 8/8**, без модели и вызова движка. Проверяющие функции не изменялись — подменён только файловый ввод. Лазейка следует непосредственно из проверки префикса schema и трёх полей свойства. [grader.py:184][grader], [grader.py:199][grader]

Это не только возможность фабрикации:

- В сохранённой переградации E6 assertion «a CTL property verified» прошёл по **`check-1-sanity.json: property 'deadlock'`**. [E6/with_skill/grading.json:44][grade6]
- В E7 assertion «The weak-fairness run’s report» прошёл по **`check-1-safety-reach.json: property 'deadlock'`**. [E7/with_skill/grading.json:54][grade7]
- E2 также требует лишь любое `verified/exhaustive`, без delivery-формулы. [evals.json:126][eval]

Все **32** сохранённых grading iteration-2/3/4 воспроизвелись по действующим assertions. Поэтому пересчёт арифметически корректен, но его «100%» не означает проверки заявленных CTL/LTL/fairness-свойств. Кроме того, сами ответы не перезапускались с исправленным skill — это прямо оговорено в подтверждении. [review-fixes-confirmation.md:98][fix]

Нужна связка: доверенный журнал вызова → вход/хеш модели → конкретные kind/formula/fairness → соответствующая запись отчёта. Одного произвольного JSON из outputs недостаточно.

**Ложные провалы тоже остались.**

- E4 разрешает CTL, но отдельно требует LTL/progress и объявленное изменение модели. На корректном CTL-only ответе я получил два regex-провала. [evals.json:243][eval], [evals.json:256][eval], [evals.json:298][eval]
- E3 теперь принимает два CLI-маршрута, но не `mc_parse` → `mc_check`, хотя MCP — основной интерфейс skill. [evals.json:169][eval]
- Assertion разрешает цитирование запрещённого слова ради отрицания, но английское `Do not say proved` всё равно проваливается: исключение добавлено только для непосредственно предшествующего русского `не `. Проверил функцию отдельно. [evals.json:66][eval]

**Eval 8 и эталон A7.**

Сам `handshake.pml` годится как **одна конкретная формализация**: два клиента, rendezvous-каналы, сервер не принимает следующий запрос до release. [handshake.pml:12][handshake], [handshake.pml:17][handshake], [handshake.pml:35][handshake] Поставленный бинарник подтвердил заявленные `deadlock` и `assert`: `verified/exhaustive`, 12 состояний.

При этом:

1. `grant(id)` и `rel(who)` при приёме **записывают** переменную, а не проверяют равенство прежнему ID. Frontend различает `Match` и `Var`. [frontend/promela/lower.go:1244][lower] Здесь это не ломает протокол: rendezvous и последовательность сервера оставляют только одного ожидающего разрешения клиента. Но модель нельзя считать эталоном проверки адресации.
2. `deadlock` отвечает на глобальный тупик, а не на голодание отдельного клиента. В prompt «может зависнуть» эту трактовку необходимо назвать. [evals.json:540][eval], [handshake.pml:21][handshake]
3. Assertions **не требуют результата mutex-проверки**: слово `assert` засчитывает постановку вопроса, а обе JSON-проверки может удовлетворить один `deadlock: verified`. [evals.json:562][eval], [evals.json:569][eval], [evals.json:577][eval]
4. Сравнения с ручной моделью нет. Поле `fixture` не используется проверками E8; требование «модель показана» удовлетворяет одно слово `proctype`. [evals.json:543][eval], [evals.json:551][eval]
5. Это ещё не проверка исходной A7: план обещает способность агента **порождать IR**, тогда как expected_output предпочитает Promela, а прямой IR оставляет experimental. [14-skill-building-plan.md:305][plan], [evals.json:541][eval]

Итого: эталон исполним, но **assertions не отвечают на вопрос, корректно ли агент построил и проверил модель по словам**. В рассматриваемых коммитах нет и измеренного агентского E8: соответствующая проверка помечена `@pending`. [g6-package.feature:146][g6feature]

## 5. Чего по-прежнему нет

По важности:

1. **Согласованного контракта интерпретации результатов.** Остались противоположные определения bounded, форм трассы, полей temporal и отказов. R1 этого не проверяет: реализация — `strings.Contains`, а сценарий «факт вызова движка» требует лишь наличия строки `"type": "engine_report"`. [steps_r1_test.go:49][r1code], [r1-review-fixes.feature:121][r1] Нужны проверки конкретных результатов и полная сверка дублирующих инструкций.

2. **Доказанного сквозного агентского маршрута через установленный MCP и надёжного измерения исправленного skill.** Переградация старых ответов этого не заменяет. Последнее зафиксированное состояние — «Связка “скилл → MCP” целиком проверена не была». [g6-confirmation.md:513][g6] E8, триггеры и новые assertions требуют отдельной приёмки на фиксированной версии.

3. **Завершённого нормативного перехода от требований к принятому продукту.** FR-009/010 всё ещё требуют прежние backends; A7 — direct IR; план по-прежнему отождествляет IR с VerificationCase. [11-skill-requirements.md:346][req], [14-skill-building-plan.md:305][plan], [14-skill-building-plan.md:137][plan] Карточка задачи, происхождение допущений и исполняемый граф должны иметь разные контракты.

4. **Убедительного основания обобщать доверие и размерные оценки.** A1 всё ещё предлагает при сомнении в корректности понижать evidence до `bounded/experimental`; это не исправляет возможную несостоятельность поиска. A2 всё ещё объявляет учебные корпуса репрезентативными без определения пользовательского распределения. [14-skill-building-plan.md:299][plan], [14-skill-building-plan.md:300][plan] Оговорка A4 теперь честнее, но область гарантий для temporal-произведений не установлена — сама reference признаёт таблицу «target, not a guarantee». [evidence-and-status.md:163][es]

5. **Завершённой релизной приёмки и оценки полезности.** Ограничение платформ исправлено словами, остальные платформы не испытаны; прежнее ревью плана также оставляло открытыми обязательные release gates и измерение пользы против стоимости ответа. [engine-tools.md:315][et], [review-astra-plan.md:129][planreview] Эти вопросы исправлениями документации не закрываются.

**Согласование полного закрытия я не даю.** До него приоритетны: исправить новые неверные рецепты и оставшиеся противоречия; связать evals с конкретными исполненными проверками; затем провести свежую сквозную приёмку на зафиксированной версии.

[skill]: model-check-plugin/skills/model-check/SKILL.md
[es]: model-check-plugin/skills/model-check/references/evidence-and-status.md
[et]: model-check-plugin/skills/model-check/references/engine-tools.md
[cex]: model-check-plugin/skills/model-check/references/counterexamples.md
[props]: model-check-plugin/skills/model-check/references/properties-ltl-ctl.md
[pn]: model-check-plugin/skills/model-check/references/petri-nets.md
[ps]: model-check-plugin/skills/model-check/references/promela-subset.md
[fair]: model-check-plugin/skills/model-check/references/fairness.md
[wf]: model-check-plugin/skills/model-check/references/workflow.md
[pit]: model-check-plugin/skills/model-check/references/pitfalls.md
[tpl]: model-check-plugin/skills/model-check/assets/report-template.md
[eval]: model-check-plugin/skills/model-check/evals/evals.json
[fixtures]: model-check-plugin/skills/model-check/evals/fixtures/README.md
[handshake]: model-check-plugin/skills/model-check/evals/fixtures/handshake.pml
[grader]: model-check-plugin/evals-workspace/grader.py
[grade6]: model-check-plugin/evals-workspace/iteration-4/eval-6-ctl-ag-ef-idle/with_skill/grading.json
[grade7]: model-check-plugin/evals-workspace/iteration-4/eval-2b-starvation-loop/with_skill/grading.json
[fix]: model-check-plugin/steps/review-fixes-confirmation.md
[logic]: model-check-plugin/steps/review-fixes-logika.md
[planreview]: model-check-plugin/steps/review-astra-plan.md
[g6]: model-check-plugin/steps/g6-confirmation.md
[g6feature]: model-check-plugin/features/g6-package.feature
[r1]: model-check-plugin/features/r1-review-fixes.feature
[r1code]: model-check-plugin/engine/steps_r1_test.go
[plan]: model-check-skill-notes/14-skill-building-plan.md
[req]: model-check-skill-notes/11-skill-requirements.md
[check]: model-check-plugin/engine/mcp/check.go
[simulate]: model-check-plugin/engine/mcp/simulate.go
[explore]: model-check-plugin/engine/explore/explore.go
[cycle]: model-check-plugin/engine/explore/cycle.go
[ctlcheck]: model-check-plugin/engine/explore/ctlcheck.go
[ctlparse]: model-check-plugin/engine/ctl/ctl.go
[ctlnormal]: model-check-plugin/engine/ctl/normal.go
[lower]: model-check-plugin/engine/frontend/promela/lower.go
[petri]: model-check-plugin/engine/frontend/petri/petri.go
[cli]: model-check-plugin/engine/cli/cli.go
