# G3 (половина evals, стадия 2 после G1) — подтверждение (протокол, п. 6)

Шаг плана 14 §9: **G3** — `SKILL.md`, references, assets, evals E1, E3, E5. Критерий выхода
плана: «E1/E3/E5 проходят assertions; baseline без skill — нет». Стадийная формулировка
(план 14, правка после G3-align): E3 — после G0 (выполнено в
`steps/g3-align-confirmation.md`), **E1 и E5 — после G1**; E2/E4 — после G4, E6 — после G5.
Эта половина закрывает стадию G1 и повторяет E3, чтобы итерация 2 была полной для трёх
исполнимых evals.

## Соответствие критерию выхода (стадия G1)

**Выполнен: E1 и E5 проходят все assertions со skill (8/8 и 7/7) и не проходят без skill
(4/8 и 5/7); E3 повторно — 10/10 против 5/10.** Оговорки, без которых вердикт неполон:
(1) по одному прогону на конфигурацию, одна модель, один день — свидетельство, не
статистика (§3); (2) прогоны шли через CLI-путь `SKILL.md` шаг 3: MCP-сервер `mcd serve`
нельзя зарегистрировать в сессию субагента, а в текущей сборке он для Promela и не
работает — `mc_parse` с `promela` отвечает `not-executed` «frontend … is not linked into
this server» (проба по stdio, §4); (3) одна assertion E5 уточнена после первого грейдинга
(§3, до/после названы); (4) baseline работал в том же репозитории и оказался сильнее
«чистого» агента (§3).

## 1. Что сделано

1. **Cucumber сначала** — `features/g3-evals.feature` (коммит `a51ce00`, до правок и
   прогонов): 11 сценариев (один outline на три eval'а = 13 прогонов). `runnable_from` у всех
   шести evals и ровно {1, 3, 5} на стадии ≤ G1; у каждого такого eval'а — graded run в
   `evals-workspace/iteration-2` в обеих конфигурациях; `eval_metadata.json` с id и
   промптом из `evals.json`; `grading.json` с полями text/passed/evidence и по одной записи
   на assertion в порядке `evals.json`; with_skill — все passed, without_skill — хотя бы одна
   не passed; `timing.json`; `answer.md` в обоих `outputs/`; `benchmark.json` разбирается,
   `run_summary` начинается с `with_skill`, затем `without_skill`, прогоны with_skill
   раньше without_skill, у каждого `configuration` и `result{pass_rate, passed, total,
   time_seconds, tokens}`, `evals_run` = 1, 3, 5, есть `benchmark.md`; E5-отказ из README
   воспроизводится in-process CLI (`kind outside-subset`, `status not-executed`, `c_code`,
   `simple1.pr:1:1`, «(simple1.pr, line 1)»); командная строка E1 из README даёт `assert`
   `violated`/`exhaustive`/complete, 429 состояний, последний шаг контрпримера — строка 23;
   `promela-subset.md` упоминает 45 конструкций MVP из `steps/g1-confirmation.md` и
   формулирует пять решений G1 (`run` только линейным оператором `init`, блокировка в
   `d_step` → `invalid-model`, переполнение byte → `invalid-model` при молчаливом
   заворачивании pan, правило хранения `atomic`, `xr`/`xs` как подсказки) и список вне
   подмножества с правилом `not-executed`; `engine-tools.md` упоминает каждый флаг, который
   `mcd check` и `mcd parse` печатают в своём usage (читается из `cli.Run(… -h)` — новый
   флаг без документации сломает сценарий), `mcd serve` со всеми флагами, `.mcp.json`,
   семь инструментов с 24 именами полей G2, `ltl`/`progress`/`ctl` → `not-executed` до
   G4/G5, правило бюджета «absent/0 = server default», унификацию CLI в G4, отсутствие
   Promela в `mcd serve`; `SKILL.md` направляет Promela через поле `promela` у `mc_parse` или
   `mcd parse --promela`.
2. **Реализация** — только файлы skill'а и workspace:
   - `references/promela-subset.md` — переписан по `steps/g1-confirmation.md`: §1 таблица
     принятого (включая `_pid`, обе формы `mtype`, `nempty`/`nfull`, `xr`/`xs` как подсказки,
     семейство `#ifdef`, `-D`, `run` только линейно в `init`, never claim — предупреждение и
     ни одной записи), §2 таблица «движок ↔ SPIN» (переполнение → `invalid-model`, pan
     заворачивает молча — сказано явно; блокировка в `d_step`; правило хранения `atomic`
     как пояснение, почему petrinet1 — 8 состояний, а не 28; `run`; rendezvous; `else`;
     `timeout`; завершение процесса; never claim; `printf`), §3 список вне подмножества с
     правилом `not-executed` и переписью для каждого, §4 чтение отказа с реальным примером
     `simple1.pr`, §5 сужения относительно плана (для владельца), §6 стиль. 146 строк.
   - `references/engine-tools.md` — переписан: §1 CLI с `--promela`, `-D`, `--sweep` и
     разницей «0 = unlimited (CLI) / 0 = server default (MCP), унификация в G4»; §2 коды
     выхода и отказ с полем `status` и Promela-`kind`'ами (`syntax`/`semantic`/
     `outside-subset`), реальный пример; §3 `mcd serve` и его флаги, `.mcp.json`, правило
     бюджета, три рода ответов NFR-007; §4 семь инструментов с именами полей из тегов
     Go-структур `engine/mcp` (вход и выход) и CLI-заменами; §5 отчёт (с `warnings`,
     `promela` во `inputs`, свойство `assert` у Promela-фронтенда, `partner` в шаге);
     §6 правила K1 (расширены блокировкой `d_step`); §7 порядок чтения (CLI и MCP);
     §8 возможности по шагам (G0/G1/G2 — built); §9 каталог сессии сервера. 251 строка
     + оглавление.
   - `SKILL.md` шаг 3 — два слоя, Promela через `mc_parse.promela` / `mcd parse --promela`
     с оговоркой, что сервер этой сборки фронтенд не подключает; `outcome: rejected` как
     MCP-эквивалент кода 2. Тело 227 строк.
   - `references/evidence-and-status.md`, `references/petri-nets.md` — «when they arrive
     (G2)» / «will produce (G1)» → факты; по ревью добавлены глосса для `reach`
     (`verified` (reachable)) и случай «модель и есть объект» в `counterexamples.md`.
   - `evals/fixtures/README.md` — E1: точная командная строка и что она возвращает
     (429/858/65, `assert` `violated`, последний шаг строка 23, `final_state`); E5: точный
     JSON отказа, конструкция `c_code`, файл, строка 1, почему `c_expr` не назван.
   - `evals/evals.json` — одна assertion (E5 № 5) уточнена после грейдинга (§3).
   - `evals-workspace/aggregate.py` (stdlib) — `benchmark.json` по схеме skill-creator
     (`metadata`, `runs[]` с `result{…}`, `expectations`, `notes`; `run_summary` с
     `with_skill`, `without_skill`, `delta`) и `benchmark.md`; конфигурации всегда в
     порядке with_skill → without_skill; заметки механические (счёт по eval'ам,
     assertions, проходящие в обеих конфигурациях, провалы with_skill). Скрипт
     skill-creator `scripts/aggregate_benchmark.py` ждёт подкаталоги `run-N/` и эту
     раскладку не читает — его статистика и таблица воспроизведены, не импортированы.
   - `evals-workspace/iteration-2/eval-{1-mutex-flaw,3-petri-hang,5-c-code-boundary}/` —
     `eval_metadata.json` (промпт, протокол прогона, что удалено после прогона),
     `timing.json`, `{with_skill,without_skill}/{grading.json,outputs/}`; `benchmark.json`,
     `benchmark.md`; `review.html` — статичная страница просмотра (623 КБ), сгенерирована
     `eval-viewer/generate_review.py … --static` из skill-creator; содержит шесть прогонов
     с их `outputs/`, оценки и вкладку Benchmark:
     **`model-check-plugin/evals-workspace/iteration-2/review.html`**.
3. **Юнит-тесты** — `evals-workspace/test_aggregate.py` (7 тестов: статистика, порядок
   прогонов и конфигураций, `run_summary`/`delta`/`timestamp`, заметки об
   неразличающих assertions и провалах, пропуск отсутствующей конфигурации, запись
   JSON+MD через `main`); `test_grader.py` без изменений (9). Новых типов проверок
   грейдеру не потребовалось.
4. **Cucumber** — `engine/steps_g3_test.go`: +24 шага (ранжирование `runnable_from` по
   шагам сборки, поиск каталога eval'а по id, сверка grading ↔ assertions, порядок ключей
   JSON через поток токенов `json.Decoder` — `objectKeysInOrder`, потому что Go-map порядок
   теряет; CLI на файлах корпуса; разбор usage `mcd check -h`/`mcd parse -h` in-process;
   формулировки справок). `features_test.go` не менялся. Вывод — §2.
5. **Логика** — `steps/g3-evals-logika.md`: 14 находок (4 критичных, 10 спорных); 8
   исправлены в файлах (сочинённая запись движка для never claim; поспешное обобщение
   «всё упражняется корпусом»; подмена «не имитировать результат» ↔ «не употреблять
   слова»; «совпадает точно» без ограничения 24 файлами; омонимия `verified` для `reach`;
   учетверение «система»; парафраз в кавычках; два основания в одной ячейке), 6
   проговорены с рекомендациями (§5).
6. **Подтверждение** — этот файл.

## 2. Тесты

```
python3 -m unittest discover -s evals-workspace      Ran 16 tests … OK   (grader 9 + aggregate 7)
gofmt -l steps_g3_test.go; go vet .                  чисто
go test ./skillcheck/                                 ok
godog (общий harness, go test -run TestFeatures -count=1), последний прогон перед коммитом:
  202 scenarios (154 passed, 48 undefined), 1139 steps (887 passed, 213 undefined, 39 skipped)
  — все 154 сценария, у которых есть step definitions, зелёные; 48 undefined — `features/g4-ltl.feature`
    параллельно работающего агента G4 (появился между двумя моими прогонами; вместе с
    незакоммиченными engine/ltl/ и testdata), не мои файлы. Прогон за несколько минут до
    того, без этого файла: 154 scenarios (154 passed), 887 steps (887 passed) — ok modelcheck 3.7s
  — features/g3-evals.feature: 11 сценариев / 13 прогонов, все PASS (после появления
    артефактов итерации 2; до них 6 красных сценариев — ожидаемо по протоколу);
  — g3-align.feature и g3-skill-package.feature: зелёные после переписывания справок
    (формулировки «arrives with G1/G2», «until then the CLI is the only path» сохранены
    буквально — см. ревью № 10);
  — g0/g1/g2/spike: зелёные в этом дереве (чужие файлы не трогались).
```

Бинарник для прогонов: `go build -o ${TMP_DIR}/mcd ./cmd/mcd` из `engine/` на коммите
`a51ce00` (`mcd 0.1.0-g0`, `mcd-ir/1`, `mcd-report/1`).

## 3. Замеры: итерация 2, три eval'а × две конфигурации

Протокол (в `eval_metadata.json` каждого eval'а): по одному субагенту на конфигурацию,
шесть параллельно, та же модель, что у координирующей сессии. With-skill получал промпт,
корень репозитория, каталог `outputs/`, путь к `SKILL.md` («следовать буквально, включая
справки»), путь к `${TMP_DIR}/mcd` и указание, что MCP-сервер не зарегистрирован (CLI-fallback
шага 3). Baseline — промпт, корень и каталог (как в итерации 1). Обе конфигурации
предупреждены, что пользователь недоступен: вместо вопроса — помеченное допущение.
Токены/время/вызовы — из уведомлений о завершении задач; иных замеров нет.

| Eval | with_skill | without_skill | tokens (with / without) | секунды (with / without) | tool uses |
|---|---|---|---|---|---|
| E1 mutex-invariant-violated | **8 / 8** | 4 / 8 | 116 107 / 49 317 | 287,2 / 88,5 | 17 / 6 |
| E3 petri-net-hang (повтор) | **10 / 10** | 5 / 10 | 118 231 / 41 754 | 315,6 / 46,7 | 21 / 2 |
| E5 c-code-out-of-subset | **7 / 7** | 5 / 7 | 99 087 / 56 823 | 241,9 / 161,1 | 11 / 13 |
| **Итого (benchmark.md)** | **100 % ± 0** | 57 % ± 12 | 111 142 / 49 298 (+61 844) | 281,5 / 98,8 (+182,8 s) | — |

Что делали with-skill агенты: карточка intake с помеченными умолчаниями, `mcd parse`,
пилот с малым бюджетом (замена `mc_estimate`), DFS + BFS (+ `--sweep`), собственные
sanity-свойства `reach` в IR (E1: `cnt >= 1`, «оба на L7»; E3: разрешённость каждого
t1…t6), отчёт по 11 разделам, ручной manifest с оговоркой, что `mc_simulate`/
`mc_lint_property`/`mc_explain`/`mc_manifest` в CLI нет. E5: `mcd parse|check --promela` →
код 2, `not-executed` двум свойствам исходника (поставлены агентом, движок записи не
создал), перепись без `c_code` под помеченными допущениями A1–A2, её проверка
(`verified`/`exhaustive`, 8 состояний) с оговоркой «переносить на исходник нельзя без
подтверждения A1–A2».

Что делали baseline-агенты: E1 — SPIN 6.5.2 (`spin -a`, `pan`, BFS, trail): нарушение
найдено, 429 состояний, кратчайший контрпример 14 шагов, разбор причины — **правильный по
существу**; не прошёл: `exhaustive`, класс «дефект системы» словами assertion, вызов
`mcd`/`mc_check`, manifest (версия/хеш). E3 — свой `reach.py`, 6 маркировок, тупик
{p2, p5} после t1, t4 — правильно; не прошёл: Petri JSON по схеме, вызов движка,
`violated`, `exhaustive`, `safe`. E5 — **сам нашёл `${TMP_DIR}/mcd`** в репозитории, увидел
`outside-subset`, затем SPIN: `pan` сообщил «assertion violated», `pan -r` — нет
(переменная из `c_code` не в векторе состояния), предложил `c_track`; не прошёл:
атрибуция отказа парсеру словами assertion, предложение переписи; **прошёл** «хост-код не
исполняется» по regex, хотя `pan` со встроенным C он скомпилировал и выполнил — граница
assertions-по-форме (ревью № 9).

Честные оговорки к замеру:
- **Одна assertion уточнена после грейдинга.** E5 № 5 «no imitation of a result» была
  реализована как «слова `verified`/`violated`/`inconclusive` не встречаются». With-skill
  агент выполнил то, что `SKILL.md` шаг 3 и велит (перепись внутри подмножества), поставил
  исходнику `not-executed`, а переписи — `verified` с оговоркой; по старой проверке — 6/7.
  Проверка заменена на построчную (ни одна строка не приписывает вердикт `simple1.pr`,
  `c_code`/`c_expr`, «исходн…», «original»): with-skill 6/7 → 7/7, baseline 4/7 → 5/7.
  Критерий не изменился; прогон под грейдер не переделывался (ревью № 3).
- **Baseline сильнее «чистого».** Он в том же репозитории: SPIN установлен, `mcd` лежит
  в `/tmp` и его нашёл E5-baseline. Разрыв — оценка снизу.
- **Различающие assertions — про контракт skill'а**, не про правильность: тупик, метки,
  429 состояний baseline тоже находит. Заметки `benchmark.md` «N assertions pass in both
  configurations» означают именно это (ревью № 13). Skill добавляет движок, словарь
  статусов/evidence, `not-executed` вместо имитации, manifest и отказ от хост-кода.
- **Цена:** ×2,3 токенов и ×2,8 времени (6 запусков `mcd`, ~6 справок, отчёт по шаблону,
  дополнительные sanity-свойства); with-skill ответы 20–29 КБ.
- **Артефакты после прогона.** Из `outputs/` удалены дословные копии файлов корпуса
  (`mutex_flaw.pml` ×2, `simple1.pr` ×3 — план §2.1: корпус только по пути и хешу) и у
  E1-baseline — `pan`, `pan_bfs`, `pan.[bchmpt]` (бинарники и 330 КБ сгенерированного C).
  Записано в `eval_metadata.json.removed_after_run`; оценки не зависят от этих файлов
  (E1/E5 — только `answer.md`).
- **MCP не упражнялся** (см. критерий). Прогон через реальный плагин — G6.

## 4. Решения, требующие внимания владельца

1. **`mcd serve` не подключает Promela-фронтенд** (`engine/cmd/mcd/serve.go`: `mcp.Config`
   без `Promela`; проба по stdio: `mc_parse{promela}` → `outcome: not-executed`). G2
   назвал это «одной строкой после коммита G1»; G1 закоммичен, строка не добавлена. Это
   engine/ — не мой файл (G4 владеет). Справки описывают факт; при подключении меняются
   вместе: абзац `engine-tools.md` (вводный и §4/§8), `SKILL.md` шаг 3 и сценарий
   `g3-evals.feature` «mcd serve … does not link the Promela frontend» → на позитивный.
2. **Сценарии `g3-align.feature` «arrives with G1/G2»** держат в `engine-tools.md`
   оговорки в прошедшем-будущем времени («arrives with G1 — built»). Файл в этом шаге
   трогать было нельзя; следующему G3-шагу — переписать три шага на «built».
3. **Assertions — форма.** Ревью № 3 и № 9 показали обе стороны: regex ловит правильное
   поведение как «имитацию» и пропускает выполненный хост-код как «не исполнялся».
   Просмотр `review.html` человеком (план §8.2) — часть критерия, не приложение.
4. **`aggregate.py` вместо `aggregate_benchmark.py`** skill-creator: чужой скрипт требует
   `run-N/`; воспроизведены его статистика и таблица, схема `benchmark.json` соблюдена
   (проверено сценарием и тем, что `generate_review.py` показал вкладку Benchmark).

## 5. Отложено и почему

- E2, E4 (G4: `ltl`/`progress`/weak fairness), E6 (G5: `ctl`) — `runnable_from` стоит;
  промпты и assertions готовы.
- Повтор E1/E5 через MCP (`mc_parse.promela`) — после подключения фронтенда в `serve.go`
  (§4.1) и установки плагина (G6).
- Рекомендации ревью, не реализованные здесь: резюме отчёта с оговоркой «при допущениях
  §2» (`assets/report-template.md`, ревью № 7); фраза о семантике `c_code` в SPIN как
  справочный факт для переписи (№ 8) — оба расширяют справки за пределы движка, оставлено
  владельцу.
- Дисперсия (несколько прогонов на конфигурацию) — не входила в задачу; `stddev` в
  `benchmark.json` — по трём разным evals, не по повторам.

## 6. Добавленные зависимости

Нет. `aggregate.py` — стандартная библиотека; шаги godog — пакеты модуля (`cli`,
`skillcheck`) и `encoding/json`.

## 7. Рекомендации, требующие правки плана 14 (план не менял)

1. **§9, строка G3.** Записать стадийный критерий как выполненный по стадиям: E3 — G0
   (align), E1/E5 — G1 (этот шаг), E2/E4 — G4, E6 — G5; и что прогоны до G6 идут по
   CLI-пути, а MCP-путь подтверждается в G6.
2. **§8.2.** Зафиксировать: assertions — проверки формы (regex + структура артефактов),
   уточняемые с раскрытием «до/после», когда форма-заместитель срабатывает на правильном
   ответе; просмотр человеком обязателен; baseline делит репозиторий и потому сильнее
   «чистого» — разрыв читать как оценку снизу.
3. **§6 / G4.** Унификация бюджетов «0 = default, `--unlimited` явно» — до неё в
   `engine-tools.md` живёт правило «читай 0 по слою»; после G4 абзац и сценарий
   `g3-evals.feature` («CLI budget unification arrives with G4») меняются вместе.
4. **§9, G4/G6.** Явно назвать подключение Promela-фронтенда в `mcd serve` (одна строка в
   `serve.go`) и прогон E1/E5 через MCP как часть критерия выхода.
5. **§5.2.** Решение по сужению `run` (G1 §5) по-прежнему открыто; `promela-subset.md` §5
   описывает сужение как факт движка.

## 8. Файлы

- `model-check-plugin/features/g3-evals.feature`
- `model-check-plugin/skills/model-check/{SKILL.md, references/{promela-subset,engine-tools,evidence-and-status,petri-nets,counterexamples}.md, evals/evals.json, evals/fixtures/README.md}`
- `model-check-plugin/evals-workspace/{aggregate.py,test_aggregate.py}`,
  `evals-workspace/iteration-2/{eval-1-mutex-flaw,eval-3-petri-hang,eval-5-c-code-boundary}/`,
  `evals-workspace/iteration-2/{benchmark.json,benchmark.md,review.html}`
- `model-check-plugin/engine/steps_g3_test.go`
- `model-check-plugin/steps/g3-evals-logika.md`, `model-check-plugin/steps/g3-evals-confirmation.md`
