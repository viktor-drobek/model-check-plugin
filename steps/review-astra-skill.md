# Внешнее ревью собранного skill — Codex (gpt-6-astra)

Дата: 2026-09-29. Инструмент: `codex exec --json -s read-only --json resume <thread>`,
модель `gpt-6-astra`, reasoning effort `xhigh`, тот же тред, что и у
[review-astra-plan.md](review-astra-plan.md), то есть ревью скила сделано с уже прочитанным
планом и подтверждениями шагов.

Предмет: `skills/model-check/` (SKILL.md, десять references, assets, evals, fixtures),
`.claude-plugin/plugin.json`, `mcp/servers.json`; код движка и `features/` — выборочно под
каждое проверяемое утверждение.

Рецензент движок не запускал. Единственный исполненный эксперимент — вызов функций
`evals-workspace/grader.py` в памяти на искусственных ответах (раздел D).

Ниже — ответ рецензента дословно; пути в ссылках приведены к путям от корня репозитория.

---

Skill пока не согласован с текущим движком: он одновременно запрещает работающие возможности и обещает отсутствующие свидетельства проверки.

Прочитал SKILL.md, **все десять** references, assets, evals и fixtures, манифест и MCP-конфигурацию; код и features — выборочно по проверяемым утверждениям. Движок и платформенные сборки не запускал. Единственный исполняемый эксперимент — вызов функций grader в памяти, без записи файлов. Ничего не менял.

## A. Соответствие плану и требованиям

**Структура §7 выполнена; содержательный контракт — нет.** В плане тоже десять references, а не девять: [14-skill-building-plan.md:175][plan] перечисляет файлы от `workflow.md` до `pitfalls.md`. Восемь шагов SKILL.md соответствуют [плану:202][plan]: «Intake» → «Модель → IR» → «Проверка» → «Отчёт». Petri-схема побайтно совпадает с движковой — проверил непосредственно.

Незакрытые обещания:

- **FR-015 — воспроизведение контрпримера.** Требование: «Воспроизводить и декодировать контрпример» — [11-skill-requirements.md:352][req]. Workflow объявляет это выполненным после `mc_explain`, который лишь читает сохранённую трассу. Подробнее B.5.
- **FR-012/NFR-002 — воспроизводимость.** Workflow обещает передать карточку, mapping и свойства в manifest: «All are inputs to `mc_manifest`» — [workflow.md:105][wf]. У инструмента единственное входное поле — `session_id`; параметры вызова не сохраняют переданные выражения свойств. Подробнее C.
- **FR-014/FR-016/NFR-001 — смысл статуса и evidence.** Слова из словаря присутствуют, но `reach`, отсутствие контрпримера и границы `bounded` описаны неверно. Наличие нужных токенов не означает выполнение требований: [features/g3-skill-package.feature:69][fg3] проверяет именно «exactly these statuses».
- **Живость переходов Petri не доведена до исполнимого рецепта.** План обещает «при запросе — живость переходов» и оговаривает замену `fire(t)` на `enabled(t)` — [план:147][plan]. Reference остался на «today neither can be run» и не объясняет актуальный прокси — [petri-nets.md:144][pn].

Изменения относительно плана:

- Добавлен полезный **E2b/starvation**, которого нет в таблице E1–E6: [evals.json:425][eval] — `"variant_of": 2`. При этом план всё ещё ожидает от E2 результат, «зависящий от fairness», а актуальный eval правильно требует независимость на lock-step модели: [план:230][plan], [evals.json:76][eval].
- Ручной вывод `violated` при strong fairness — дополнительное правило skill, отсутствующее в плане, где сказано «strong fairness — не поддерживается»: [fairness.md:143][fair], [план:117][plan].
- **CTL, `provided`, `inline`, `typedef`, динамический `run` не являются самовольным расширением:** они предусмотрены v1 — [план:118][plan], [план:141][plan]. Перенос конфигурации в `mcp/servers.json` уже внесён в текущий план — [план:96][plan]; старое замечание G6 об этом больше не актуально.
- Буквальные FR-009/FR-010 из 11 требуют внешних SPIN/NuSMV workflows. Это сознательно изменённый владельцем scope, а не дефект реализации. Однако заявлять полное соответствие неизменённому 11 нельзя: [11:346][req] — «Поддерживать SPIN/Promela workflow», «NuSMV-подобный workflow».

## B. Честность

### B.1. CTL одновременно разрешён и запрещён

**Skill:** [SKILL.md:266][skill] — «It does not check CTL yet»; [properties-ltl-ctl.md:135][props] — «this build does not check CTL at all»; [evidence-and-status.md:125][es] предлагает причину «kind `ctl` not executed by this build».

**Факт:** [ctlcheck.go:115][ctlcheck] присваивает `Verified`, а при ложности — `Violated`; [features/g5-ctl-v1.feature:105][fg5] фиксирует `AG EF subscriber@Idle` → «verified … exhaustive».

**Вывод:** агент может отказаться от поддерживаемой проверки. Отдельно устарел пример Busy: «label atoms are not accepted» — [SKILL.md:255][skill]. Это ограничение LTL, не CTL.

### B.2. Два примера CTL-синтаксиса движок не принимает

**Skill:** [properties-ltl-ctl.md:48][props] обещает `proc[i]@label`; [там же:74][props] приводит `E(p U q)`, а таблица — `A(p U q)`.

**Код:** [ctl/ctl.go:585][ctlparse] — «remote parses `P@label` and `P:pid@label`»; ветка until требует именно `[` — [ctl/ctl.go:483][ctlparse], с диагностикой «expected U inside … `[ … U … ]`».

**Вывод:** агенту даны неисполняемые образцы. Нужны `P:pid@label`, `E[p U q]`, `A[p U q]`, а также пояснение выбора экземпляра: без номера имя proctype означает первый экземпляр — [g5-confirmation.md:30][g5].

### B.3. Перевёрнута регрессионная проверка достижимости

**Skill:** [counterexamples.md:178][cex] предлагает сохранить `reach bad`, а после исправления получить «`verified` on `[] !bad` (unreachable)».

**Код:** [explore.go:1765][explore] для недостижимого `reach` возвращает `Violated` и «no reachable state satisfies … (complete search)».

**Вывод:** инструкция смешивает два разных свойства. После исправления `reach bad` должен стать `violated`; `verified` относится к отдельному invariant `!bad` либо LTL `[] !bad`.

Та же ошибка сокращения в [SKILL.md:204][skill]: «verified — search completed, no violation found». Для `reach` достаточно найденного свидетеля, и обход может быть незавершённым: [report.go:236][report] — «completeness is not needed».

### B.4. Обещаны контрпримеры и деревья, которых нет

**Skill:**

- [SKILL.md:174][skill]: «For every `violated` property call `mc_explain`».
- [counterexamples.md:32][cex]: для вложенного CTL «returns the path … and, per branch, why it fails».
- [engine-tools.md:211][et]: counterexample «present for `violated`».

**Код:** [ctl/label.go:258][ctllabel] — «a failing existential property has no run to show»; [там же:265][ctllabel] — объяснение вложенного отказа «is not shown». Недостижимый `reach` также завершается без трассы — [explore.go:1765][explore].

**Вывод:** обязательный вызов невозможно сформировать без `counterexample_id`; дерево CTL движок не строит. Нужно ветвление по наличию `counterexample`/`witness` и обработка `temporal.witness_note`.

### B.5. Форматирование трассы названо replay

**Skill:** [workflow.md:120][wf] — «`mc_explain` replayed the trace». [counterexamples.md:141][cex] обещает replay через `mc_simulate` «from the counterexample id».

**Код:** [mcp/check.go:460][mcheck] делает `sess.ReadFile`, затем `json.Unmarshal`; далее разбивает сохранённые шаги на prefix/loop. [simulate.go:31][sim] принимает **`edges []string`**, а не ID контрпримера; при отсутствии списка отвечает «guided mode requires a non-empty edges list» — [simulate.go:88][sim].

**Вывод:** декодирование выдано за повторное исполнение. Автоматического маршрута «ID → проверенный replay» в этом контракте нет; нельзя писать «counterexample replayed» лишь после `mc_explain`.

### B.6. Формы LTL-свидетельств описаны взаимоисключающе

**Skill:** [SKILL.md:175][skill] — «A temporal violation is a lasso»; [counterexamples.md:28][cex] — для safety-формулы «there is no `loop` at all».

**Факт:** [features/g4-ltl.feature:85][fg4] фиксирует temporal `never: violated`, у которого «counterexample … has no loop». Обратный случай тоже задокументирован: собственный автомат для safety-части возвращает «acceptance cycle» — [g4-confirmation.md:95][g4].

**Вывод:** ни один из двух универсальных тезисов неверен. Форму следует читать из конкретного результата, а не выводить только из класса свойства.

### B.7. Strong fairness: ограничение честное, инструкция после него — нет

**Skill:** [fairness.md:59][fair] правильно говорит «Strong fairness is not supported». Но [там же:143][fair] после ручного просмотра лассо разрешает «`violated` for the strong-fairness reading».

**Код:** [cycle.go:285][cycle] безусловно возвращает для strong `NotExecuted, EvUnknown`.

**Вывод:** ручной вывод подменяет статус движка, вопреки собственному правилу «from the engine, unchanged» — [evidence-and-status.md:24][es]. Его можно изложить отдельно как проверенный вручную аргумент, сохранив `not-executed` для запуска.

Дополнительно [fairness.md:150][fair] предлагает «a bounded counter of consecutive skips» как явное моделирование strong fairness. Но собственное определение на [строке 23][fair] не задаёт конечного предела ожидания. Такой счётчик вводит **более сильное ограничение**, а не эквивалентную реализацию fairness; потерю поведений надо назвать.

### B.8. `bounded` превращён в необоснованное обещание покрытия

**Skill:** [evidence-and-status.md:92][es] — «everything up to depth D»; [там же:54][es] — «covered everything up to the named bound».

**Код:** DFS пропускает уже встреченное состояние — `if !isNew { continue }`, [explore.go:1505][explore]; состояния за пределом глубины уже сохранены, но не раскрываются — [explore.go:1520][explore]. При повторном достижении коротким путём они не раскрываются заново. Лимит числа состояний просто останавливает добавление следующего состояния — [explore.go:1376][explore].

**Вывод:** из такого DFS нельзя обещать проверку всех путей длины ≤ D; N сохранённых состояний также не задаёт полный поведенческий горизонт. Это вывод из управления обходом, отдельный динамический тест я не запускал. Токен `bounded` можно сохранить по контракту, но текст должен описывать фактическую неполноту.

Есть и прямой остаток старого контракта: таблица на [строке 82][es] относит time/memory stop к `bounded`, хотя ниже на [строке 94][es] правильно требует `unknown`.

### B.9. Неправильно классифицированы ошибки входа и заполненный канал

**Skill:** [SKILL.md:209][skill] включает «undefined atom» в `invalid-model`; [promela-subset.md:132][ps] утверждает для коллизии имён «the status is not `not-executed`».

**Код:** [cli.go:345][cli] прямо относит undeclared variable в свойстве к отвергнутому входу; [cli.go:353][cli] возвращает `Status: "not-executed"`.

**Вывод:** причина отказа действительно может быть ошибкой модели, но verification status при незапущенной проверке остаётся `not-executed`.

Ещё [promela-subset.md:61][ps] объединяет byte overflow и «channel capacity exceeded» под `invalid-model`. Для обычной отправки в заполненный канал [explore.go:818][explore] возвращает enabled=false: `ChanLen < Capacity`. Нужно явно отличить штатную блокировку отправки от переполнения числового домена/места Petri.

### B.10. Собственный список свойств убирает автоматические проверки

**Skill:** [SKILL.md:155][skill] требует передать property list; [properties-ltl-ctl.md:216][props] обещает, что `progress` «appears in every report … whether or not you asked for it».

**Код:** [mcp/check.go:214][mcheck] выполняет `mm.Properties = props`. Из неуказанных свойств отдельно гарантирован только implicit `assert` — [mcp/check.go:47][mcheck].

**Вывод:** добавив обязательный sanity `reach`, агент может потерять `deadlock`, `safe`, `never`, `accept`, `progress`. В подробной таблице [engine-tools.md:157][et] замена описана правильно, но основной workflow не требует объединить списки.

### B.11. `mc_lint_property` обещано больше, чем реализовано

**Skill:** [SKILL.md:114][skill] — «on every property»; [properties-ltl-ctl.md:157][props] обещает polarity note для рукописного never claim; [строка 156][props] — сведения об атомах «on all reachable states».

**Код:** [lint.go:60][lint] принимает только invariant/reach/ltl/ctl; progress отвергает, для LTL требует непустую формулу. [lint.go:286][lint] лишь рекомендует «check it with a reach property».

**Вывод:** lint не анализирует произвольный never claim на соответствие намерению пользователя и не выполняет поиск достижимости. Проверки атомов по полному графу относятся к `mc_check`, например [ctlcheck.go:136][ctlcheck]. Инструкция должна разделить синтаксический lint, ручную проверку полярности и отдельную sanity-проверку.

### B.12. Petri: несуществующий `fire(t)` и устаревший запрет temporal-проверок

**Skill:** [petri-nets.md:133][pn] предлагает `AG EF fire(t)` и `[]<> fire(t)` как доступные G5/G4 свойства, но на [строке 144][pn] говорит «today neither can be run».

**Код:** [petri.go:250][petrigo] генерирует только `deadlock` и `safe`; [petri.go:229][petrigo] строит guard из входных мест, без переменной события. LTL-парсер перечисляет допустимые функции на [ltl.go:551][ltlparse]; `fire` среди них нет.

**Вывод:** проверять temporal-формулы над маркировками можно; буквально предлагаемые формулы исполнить нельзя. Нужен рецепт построения выражения enabled и явная оговорка плана, что для LTL оно не означает фактическое срабатывание перехода.

### B.13. LTL evidence: устаревшее исключение и преувеличенная проверка оракулом

**Skill:** [workflow.md:119][wf] допускает `verified` с «explicitly flagged experimental LTL»; шаблон требует учитывать «experimental LTL evidence» — [report-template.md:74][tpl].

**Код:** [cycle.go:1083][cycle] выдаёт `Verified, Exhaustive`; [report.go:251][report] отвергает `verified` с любым другим evidence.

**Вывод:** экспериментальная ветка текущему движку не соответствует.

При этом объяснение отмены экспериментальности тоже неточно: [evidence-and-status.md:105][es] заявляет совпадение «verdicts and product state counts» в 43 тройках. Подтверждение прямо исключает сравнение счётчиков под `-f` и собственным автоматом — [g4-confirmation.md:95][g4]: «тоже не сравнивается». Это завышение доказательной базы, а не основание самостоятельно менять нынешние статусы.

### B.14. Схема результата описана с ошибками

- **Счётчики.** [engine-tools.md:207][et]: «the same for every property». [report.go:208][report] заменяет их на собственные `o.Stats` temporal-проверки.
- **Обязательные поля.** [properties-ltl-ctl.md:65][props]: «Every temporal property record carries» `stutter_invariant`. [cycle.go:313][cycle] заполняет его только для скомпилированной LTL-формулы; [report.go:149][report] допускает отсутствие через `omitempty`.
- **Источник свойства.** [engine-tools.md:209][et] называет `source: claim`; [cycle.go:326][cycle] выдаёт `never-claim`, а следующая ветка — `accept-labels`.
- **LTL без formula.** [engine-tools.md:157][et] называет formula обязательной и для LTL. [mcp/check.go:39][mcheck] допускает её отсутствие для never claim/accept labels.

**Вывод:** агент будет искать отсутствующие поля, неверно сравнивать счётчики или отклонять законный запрос.

### B.15. Обещание побайтной воспроизводимости чрезмерно

**Skill:** [engine-tools.md:275][et] — `--no-timing` «makes it byte-for-byte equal»; любое другое различие объявляется дефектом движка.

**Код:** [cli.go:338][cli] продолжает применять wall-clock timeout независимо от `--no-timing`; [report.go:201][report] этот флаг лишь убирает `time_ms`.

**Вывод:** при остановке по времени могут различаться исследованные состояния, найденные результаты и трассы. Исключение нужно написать явно.

### B.16. A4, A6, A7

**A4 — размер.** [evidence-and-status.md:135][es] допускает small без ограничения вектора; [estimate.go:246][estimate] требует ≤128 байт и для small. Кроме того, обещание предупреждения заранее — [evidence-and-status.md:144][es], «will warn before the run» — шире проверенного: [g5-confirmation.md:417][g5] признаёт «не считает память на состояние». Измерение 10⁶ состояний относится к простому counters, а не к произвольному temporal-произведению. Оговорка о простых guards в reference есть; считать таблицу гарантией 60 с/1 ГБ всё равно нельзя. Шаблон также просит «peak memory» — [report-template.md:50][tpl], тогда как JSON содержит `memory_bytes_est` — [report.go:176][report].

**A6 — платформы.** [engine-tools.md:311][et] объединяет «cross-platform binary build» и «validated with a real client» без платформенной оговорки. [g6-confirmation.md:31][g6] показывает запуск только linux/arm64; Windows wrapper не проверялся. Действующий [servers.json:5][config] указывает на `engine/bin/mcd`, начинающийся с `#!/bin/sh` — [engine/bin/mcd:1][wrapper]. Вывод: подтверждён POSIX-маршрут на одной платформе; нативный Windows-маршрут конфигурацией не выбран и здесь не проверен. Эту границу нужно сообщить пользователю skill.

**A7 — существенного ложного обещания не нашёл.** [SKILL.md:81][skill] правильно предпочитает Promela и называет direct IR «experimental». В confirmations нашёл упоминание A7 только в оценке триггеров — [g6-confirmation.md:273][g6]; это не обещанный сравнительный eval генерации IR. Пробел проверки остаётся, но skill его не маскирует утверждением о готовности.

**Promela в целом:** актуальная reference правильно учитывает расширения G5 и различие `pc_value`. Однако [SKILL.md:104][skill] и [engine-tools.md:80][et] ещё предлагают ожидать «never claim … not executed», тогда как frontend создаёт исполняемое LTL-свойство — [lower.go:474][lower]. Пример Pathfinder также оставлен в условном будущем «until then … not-executed» — [SKILL.md:250][skill], хотя [promela-subset.md:124][ps] уже говорит «inside the subset».

## C. Качество инструмента для агента

**Description перегружен и задаёт противоречивые условия активации.** [SKILL.md:14][skill] требует срабатывать на «any» из слов, включая «гонка» и «инвариант», но [строка 21][skill] исключает data-race linters и доказательства отдельных функций. Это конфликт правила по ключевому слову с правилом по намерению. Подтверждения не доказывают пользу длинного описания: в [G6:294][g6] полная и однострочная версии получили одинаковый результат; реальный headless Claude не участвовал — «Not logged in», [G6:259][g6].

**Прогрессивное раскрытие есть по файлам, но нарушено дублированием контракта.** По подсчёту whitespace-слов: SKILL.md — 2641, references — 25 629. Проблема не в одном пороге объёма: статусы повторяются в SKILL, evidence, workflow и engine-tools и уже противоречат друг другу. Приёмка контролирует лишь «at most 500 lines» — [g3-skill-package.feature:42][fg3]. Часть обязательной справки вынесена вообще за пакет skill: [properties-ltl-ctl.md:83][props] отправляет за контрактом в steps/features и признаёт «has not yet had its row-by-row pass».

**Сквозной путь требует догадок в нескольких местах:**

- **Manifest не принимает обещанные артефакты.** «All are inputs» — [workflow.md:105][wf] — против единственного `SessionID` в [mcp/check.go:502][mcheck]. Более того, [session.go:250][session] сохраняет search/fairness/budget/seed, но не property expressions; [report.go:112][report] тоже не содержит `expr` state-свойства. Нужно отдельно сохранять точный запрос `mc_check`, свойства, intake и mapping, а не считать manifest достаточным.
- **Передача IR теряет макросы.** [SKILL.md:155][skill] говорит «Pass the IR»; [mcp/check.go:245][mcheck] при inline IR делает `defines = nil`. Для формул с Promela `#define` нужен `session_id` либо заранее раскрытая формула.
- **Дерево не покрывает нормальные исходы.** [workflow.md:89][wf] велит запустить liveness и «look at the lasso»; дальнейшие ветки описывают только существующее лассо. Нет веток для `verified`, budget stop, rejection. Это противоречит заявлению «exactly one» исход на узел — [workflow.md:15][wf].
- **CLI-fallback не эквивалентен MCP.** [SKILL.md:93][skill] говорит «every `mc_*` … CLI equivalent», но [engine-tools.md:178][et] признаёт пропуск simulation и ручной lint. Для estimate предлагается грубый малый запуск — [строка 180][et], хотя [cli.go:280][cli] уже реализует `--estimate`. Деградации должны быть перечислены в основном маршруте.

При отсутствии бинарника честный выход описан: «engine binary unavailable» → `not-executed`, [engine-tools.md:82][et]. При parser rejection тоже есть конкретный маршрут. Их портят именно противоречия классификации из B.9. При бюджете увеличение предусмотрено, но обещание полного bounded-покрытия из B.8 следует убрать.

## D. Evals

**Полный балл можно получить без работающего движка. Я это проверил.**

Вызвал `grade()` в памяти с искусственными короткими строками вместо ответов агента. Для E3 указал уже существующий каталог fixtures как outputs. Результат: **59/59**, без единого вызова `mcd`, MCP и без создания файлов.

Почему это возможно:

- 48 assertions — `regex`, восемь — `not_regex`, две — `regex_order`, одна — `petri_json_valid`. Проверок `json_field` в текущем наборе нет.
- «The engine was invoked» означает поиск текста `mcd check|mc_check` — [evals.json:50][eval]; реализация — обычный `re.search` по ответу, [grader.py:46][grader].
- Petri-проверка не вызывает frontend: [grader.py:75][grader] прямо говорит «checks a grader can do without the engine»; затем сравнивает JSON с известным fixture.
- Для E6 прошла строка **`CTL AG EF idle from every state without fairness path not-executed`**. Assertion принимает любой из шести статусов — [evals.json:413][eval], хотя expected_output обещает выполненную CTL-проверку.
- E4 не требует факта вызова движка и по-прежнему обучает обходу через progress: «control-label atoms are not accepted» — [evals.json:231][eval].

Набор измеряет **узнавание примера, словарь и оформление**, частично — построение Petri JSON. Ответы доступны прямо в SKILL.md и fixtures: например, [fixtures/README.md:99][fixtures] заранее сообщает verdict и счётчики E2. Он не отделяет использование skill от копирования известных результатов.

Пропущены: связь статуса с конкретным свойством и фактическим JSON, сохранность списка свойств, replay, отсутствие witness, strong/CTL fairness rejection, различные budget stops, A7, работоспособность установленного MCP-маршрута. Последняя не подтверждена и историческими прогонами: [G6:513][g6] — «Связка “скилл → MCP” целиком проверена не была».

Есть и ложные провалы: E3 требует именно `mcd check --petri` — [evals.json:166][eval], отвергая законный parse→IR→check маршрут; G6 это уже зафиксировал — [G6:380][g6].

## E. Находки по серьёзности

1. **[критично] [evidence-and-status.md:92][es], «everything up to depth D».** Превращает неполный DFS в гарантию горизонта; **заменить обещание на описание реально исследованного подграфа**.
2. **[критично] [counterexamples.md:180][cex], «`verified` … unreachable».** Инвертирует регрессионный результат; **развести `reach bad: violated` и `invariant !bad: verified`**.
3. **[критично] [workflow.md:120][wf], «`mc_explain` replayed».** Приписывает результату невыполненную валидацию; **разделить decode и replay и запретить заявлять replay без его отдельного результата**.
4. **[важно] [evals.json:50][eval], «engine was invoked» через regex.** Полный PASS не подтверждает выполнение проверки; **проверять журнал вызовов и связанные с ним реальные отчёты движка**.
5. **[важно] [SKILL.md:266][skill], «does not check CTL yet».** Блокирует поддерживаемые задачи; **синхронно удалить устаревший запрет из всех инструкций и fixtures**.
6. **[важно] [counterexamples.md:32][cex], «per branch, why it fails».** Обещает отсутствующее CTL-дерево; **описать реальные path/lasso/none и `witness_note`**.
7. **[важно] [SKILL.md:174][skill], «every `violated` … `mc_explain`».** Невыполнимо для недостижимости и части CTL; **вызывать explain только при наличии ID трассы**.
8. **[важно] [properties-ltl-ctl.md:216][props], «every report».** Агент может молча потерять автоматические свойства; **перед передачей `properties` явно объединять их с нужными свойствами frontend**.
9. **[важно] [fairness.md:143][fair], «`violated` for the strong-fairness reading».** Смешивает ручной аргумент со статусом движка; **оставлять engine status неизменным и отдельно маркировать аргумент и усиление допущений**.
10. **[важно] [SKILL.md:209][skill], «undefined atom».** Неверный статус ошибки входа; **разделить rejection→`not-executed`, runtime invalid-model и штатную блокировку канала**.
11. **[важно] [properties-ltl-ctl.md:48][props], `proc[i]@label`, и [строка 74][props], `E(p U q)`.** Предлагает отвергаемые формулы; **заменить примеры точным синтаксисом парсера**.
12. **[важно] [petri-nets.md:133][pn], `fire(t)`.** Нет исполнимой инструкции и различения события/разрешённости; **добавить рабочие выражения enabled с явной границей LTL-прокси**.
13. **[важно] [properties-ltl-ctl.md:157][props], «polarity note».** Обещает отсутствующий анализ never claim; **ограничить контракт lint и вынести семантические проверки в отдельные действия**.
14. **[важно] [workflow.md:105][wf], «All are inputs to `mc_manifest`».** Артефакты воспроизводимости могут не сохраниться; **обязать сохранять точный запрос и сопровождающие документы отдельно**.
15. **[важно] [engine-tools.md:311][et], «cross-platform … validated».** Скрывает реальную границу A6; **опубликовать матрицу собранных/запущенных платформ и правильный launcher каждой**.
16. **[важно] [engine-tools.md:275][et], «byte-for-byte equal».** Объявляет ожидаемую вариативность timeout дефектом; **ограничить обещание запусками, не остановленными по времени**.
17. **[мелочь] [engine-tools.md:207][et], «same for every property», и [строка 209][et], `source: claim`.** Искажает чтение JSON; **сверить таблицу полей с текущими структурами и ветками заполнения**.
18. **[мелочь] [report-template.md:74][tpl], «experimental LTL evidence», и [engine-tools.md:180][et], «crude stand-in».** Сохраняет отменённые ограничения; **удалить экспериментальное исключение и использовать существующий CLI `--estimate`**.
19. **[мелочь] [SKILL.md:14][skill], «any of them».** Создаёт конфликт триггеров с исключениями; **сформулировать активацию через намерение проверить поведение модели, а не отдельное слово**.

**Вердикт: нет, к выпуску в текущем виде skill не годится.** Причина — неверный контракт интерпретации результатов при формально зелёной проверке текста.

До выпуска, в порядке очереди:

1. Исправить статусы, bounded-покрытие, CTL/LTL witnesses, strong fairness и сохранение списка свойств во всех дублирующих инструкциях.
2. Сделать воспроизводимый маршрут: точные MCP/CLI-запросы, сохранение property expressions и явное различение decode/replay.
3. Заменить проверки слов проверками исполнения и пройти полный сценарий установленного skill через MCP; платформенную поддержку ограничить фактически проверенными конфигурациями.

[plan]: model-check-skill-notes/14-skill-building-plan.md
[req]: model-check-skill-notes/11-skill-requirements.md
[skill]: model-check-plugin/skills/model-check/SKILL.md
[wf]: model-check-plugin/skills/model-check/references/workflow.md
[es]: model-check-plugin/skills/model-check/references/evidence-and-status.md
[et]: model-check-plugin/skills/model-check/references/engine-tools.md
[props]: model-check-plugin/skills/model-check/references/properties-ltl-ctl.md
[fair]: model-check-plugin/skills/model-check/references/fairness.md
[cex]: model-check-plugin/skills/model-check/references/counterexamples.md
[pn]: model-check-plugin/skills/model-check/references/petri-nets.md
[ps]: model-check-plugin/skills/model-check/references/promela-subset.md
[tpl]: model-check-plugin/skills/model-check/assets/report-template.md
[eval]: model-check-plugin/skills/model-check/evals/evals.json
[fixtures]: model-check-plugin/skills/model-check/evals/fixtures/README.md
[grader]: model-check-plugin/evals-workspace/grader.py
[mcheck]: model-check-plugin/engine/mcp/check.go
[sim]: model-check-plugin/engine/mcp/simulate.go
[lint]: model-check-plugin/engine/mcp/lint.go
[session]: model-check-plugin/engine/mcp/session.go
[cli]: model-check-plugin/engine/cli/cli.go
[explore]: model-check-plugin/engine/explore/explore.go
[cycle]: model-check-plugin/engine/explore/cycle.go
[ctlcheck]: model-check-plugin/engine/explore/ctlcheck.go
[ctllabel]: model-check-plugin/engine/ctl/label.go
[ctlparse]: model-check-plugin/engine/ctl/ctl.go
[report]: model-check-plugin/engine/report/report.go
[estimate]: model-check-plugin/engine/estimate/estimate.go
[petrigo]: model-check-plugin/engine/frontend/petri/petri.go
[lower]: model-check-plugin/engine/frontend/promela/lower.go
[ltlparse]: model-check-plugin/engine/ltl/ltl.go
[g4]: model-check-plugin/steps/g4-confirmation.md
[g5]: model-check-plugin/steps/g5-confirmation.md
[g6]: model-check-plugin/steps/g6-confirmation.md
[fg3]: model-check-plugin/features/g3-skill-package.feature
[fg4]: model-check-plugin/features/g4-ltl.feature
[fg5]: model-check-plugin/features/g5-ctl-v1.feature
[config]: model-check-plugin/mcp/servers.json
[wrapper]: model-check-plugin/engine/bin/mcd

