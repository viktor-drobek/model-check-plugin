# G6 — подтверждение (протокол, п. 6)

> **Historical record.** The detailed measurements below describe the original G6
> run and intentionally retain its historical engine/version values. They are not
> the release evidence for the current plugin.

## Current release addendum — 0.2.0

The current release source commit is the exact value recorded in
`engine/bin/BUILD-INFO.json` under `source_commit`; the release-artifacts commit is
the commit that contains the tracked files under `engine/bin/`.
The tracked release contains Linux amd64/arm64, Darwin amd64/arm64, and Windows
amd64 binaries, plus the POSIX `mcd` wrapper, the Windows `mcd.cmd` wrapper, and
`mcd.exe` (a byte copy of the Windows binary: the file Windows resolves the
portable declaration's `engine/bin/mcd` command to). `BUILD-INFO.json`
reports version `0.2.0`, Go `go1.26.1`, `cgo_enabled` `0`, and the source commit
`dcc65bad6f1075e8c5955efb37ce8c86455d046f` ("Set the version to 0.2.0");
`SHA256SUMS` validates all eight generated files. `./build.sh --verify-repro`
compares the complete generated artifact directory, not only native binaries: the
release was built twice and the two builds agree on every generated artifact.

What 0.2.0 adds to 0.1.1 (the version is a minor step because the report gained an
opt-in field and a new CLI/MCP option; the default behaviour is unchanged):

- `mcd check --por` and `mc_check` `por`: partial-order reduction of the safety
  search, with a `search.reduction` record that says whether it was applied, why not,
  and how many states it reduced (`PROVENANCE.md`, "What the partial-order reduction
  (0.2.0) rests on"; `steps/perf2-confirmation.md`, `perf3-confirmation.md`,
  `perf4-confirmation.md`, which include the three cross-review rounds);
- directed buffered channels: the send end and the receive end of a buffered channel
  are separate cells, so the stages of a pipeline are explored one after the other;
- a visited set whose arena is chunked (a stored vector is never copied or moved;
  the table of 64-bit slots, kept at most half full so at least two slots per
  state, is rebuilt by rehashing when it doubles) and a search stack that doubles
  its capacity (it still copies the frames, but far less often than `append`'s
  quarter steps), which lowers the memory of large complete runs; without `--por`
  no state or transition count changes, and no verdict changes unless a memory
  budget decides the run (the reported `memory_bytes_est` is smaller, so a run
  that stopped `inconclusive` on `--budget-mem-mb` under 0.1.1 can now go on; the
  `petrinet2` golden was re-pinned for the estimate).

The current verification record is, on the Linux amd64 host (Go 1.26.1, SPIN 6.5.2):
`go test -count=1 ./...` (the engine module including the godog suite over 288
scenario definitions in 16 feature files, and `tools/pandiff` against SPIN) passes;
`go vet ./...` and `gofmt -l .` are clean; the manifest synchronization checks pass;
host `mcd version` prints `mcd 0.2.0 (ir mcd-ir/1, report mcd-report/1)`;
`sha256sum -c SHA256SUMS` reports OK for all eight files; and a Promela smoke check of
`testdata/promela/bench-indep.pml` (`-D N=4 -D K=4 --sweep`) gives `deadlock verified
exhaustive` both without `--por` (41 371 states) and with it (57 states).

Platforms **built and hashed but not executed** on this host: Linux arm64, Darwin
amd64, Darwin arm64, Windows amd64 (and the `mcd.exe` alias and `mcd.cmd` wrapper).
Only the Linux amd64 binary was run, so nothing here claims that the plugin works
on the other four platforms.

Cross-review of this release (three independent reviewers on different models, the
findings verified by an orchestrator who also rebuilt all five platforms from the
release source commit and got byte-identical binaries, `SHA256SUMS` and `BUILD-INFO.json`):
verdict "approve with changes". The documentation findings (the corpus-test claim,
the citation, the mutation-testing wording, the refusal lists, the build
instructions, the wording on the memory changes and on `mcd.exe`, a stale sentence
in the LTL reference) were corrected in the commit that follows the artifacts commit;
no Go source changed, so the binaries and checksums are those recorded above. A
second cross-review of those corrections (fresh brief, same three reviewers) found
that they had added `_pid` to the list of refusals although the frontend folds
`_pid` to a constant and only `_nr_pr` is refused, and several smaller
imprecisions (the Windows files, the wording about which commit is the source
commit, the scope of the corpus test, a memory sentence); these were corrected in
the commit after it. Not
changed in this release, because each needs a Go edit and therefore a rebuild of
every binary: the usage strings of `mcd` (`cli.go`) omit `serve`, and the schema
text of the `por` field of `mc_check` (`mcp/check.go`) does not list dynamic
channels and reads of the process table among the refusals; the `search.reduction`
reason in the report is complete. Still open: the CI workflow
(`.github/workflows/ci.yml`, outside the plugin tree) sets up Go 1.24 with
`GOTOOLCHAIN=local` while `engine/go.mod` requires 1.26.1, and no CI run exists for
this release, so no CI result is claimed here; and `go test ./...` has not been run
in a clean checkout of the public plugin repository alone, where scenarios that read
`model-check-skill-notes/` and `Promela - examples/` (monorepo directories) need
those directories.

Nothing was pushed, tagged, or deployed to the public `model-check-plugin` project
as part of this record; publishing is a separate, explicit step.

The previous release record (0.1.1) is the history of this file and of the public
tags `v0.1.0` and `v0.1.1`.

Шаг: **G6** (план 14 §9, строка G6). Критерий выхода дословно: «Все evals;
триггер-точность на held-out ≥ порога; плагин устанавливается на чистой машине без Go;
реальный клиент подтверждает, что `plugin.json` и корневой `.mcp.json` не регистрируют
сервер дважды».

**Соответствие критерию выхода: выполнен частично** — три части из четырёх выполнены
с названными границами, четвёртая («все evals») выполнена на итерации 4 в объёме
семи eval'ов при одном прогоне на конфигурацию. Разбор по частям — §6.

Машина: linux/arm64, Go 1.26.0, Claude Code 2.1.273, SPIN 6.5.2. Дата: 2026-09-26/27.
Сессия общая с параллельно работающими агентами G5 (движок) и G3 (справки), поэтому
тайминги — порядок величины, не бенчмарк.

---

## 1. Сборка и упаковка

`model-check-plugin/build.sh` — один скрипт, пять платформ, `CGO_ENABLED=0`,
`-trimpath -buildvcs=false`, фиксированная строка `-ldflags`. Никакого значения часов
ни в одном артефакте, поэтому вывод — функция дерева исходников и аргументов.

### 1.1. Таблица платформ

Сборка `./build.sh` (версия по умолчанию берётся из `plugin.json` → `0.1.0`),
`source_commit` 244aaa8:

| goos/goarch | файл | размер, байт | `file` | исполнялся здесь? |
|---|---|---:|---|---|
| linux/amd64 | `mcd-linux-amd64` | 9 093 282 | ELF 64-bit x86-64, **statically linked**, stripped | нет |
| linux/arm64 | `mcd-linux-arm64` | 8 388 770 | ELF 64-bit aarch64, **statically linked**, stripped | **да — хост** |
| darwin/amd64 | `mcd-darwin-amd64` | 9 322 704 | Mach-O 64-bit x86_64, `DYLDLINK|PIE` | нет |
| darwin/arm64 | `mcd-darwin-arm64` | 8 610 802 | Mach-O 64-bit arm64, `DYLDLINK|PIE` | нет |
| windows/amd64 | `mcd-windows-amd64.exe` | 9 358 848 | PE32+ console, x86-64, 8 sections | нет |
| — | `mcd` (обёртка, POSIX sh) | 903 | скрипт | да |
| — | `mcd.cmd` (обёртка для Windows) | 136 | скрипт | **нет** |

Сумма: `engine/bin/SHA256SUMS` — семь строк (пять бинарников и две обёртки), формат
`sha256sum`, порядок `LC_ALL=C sort`. `engine/bin/BUILD-INFO.json` — версия, `go1.26.0`,
`cgo_enabled: "0"`, флаги сборки, `version_stamp`, `source_commit`, размеры.
`source_commit` называет коммит `244aaa8`, на котором дерево стояло в момент сборки;
после него в `engine/` менялись только файлы `*_test.go` (проверено
`git diff --name-only 244aaa8..HEAD -- engine | grep -v _test.go` — пусто), поэтому
описанный набор бинарников соответствует и текущему состоянию движка.

**Чего таблица не утверждает.** Четыре из пяти бинарников здесь собраны и захешированы,
но **не запущены**: на этой машине нет ни macOS, ни Windows, ни x86-64. Утверждение
«плагин работает на пяти платформах» из этого шага **не следует** и в подтверждении не
делается; следует «сборка под пять платформ воспроизводимо получается и записана
контрольными суммами», а проверка запуском сделана для linux/arm64.

**Статичность.** Для двух ELF'ов `file` говорит `statically linked`, и `ldd
mcd-linux-arm64` отвечает `not a dynamic executable` — это проверено на хосте. Для
darwin-бинарников `file` показывает `DYLDLINK`: Go при `CGO_ENABLED=0` всё равно
связывает `libSystem` через dyld, это нормальное состояние Go-бинарника под macOS, а не
след C-зависимости; проверить это запуском здесь нельзя. Для PE32+ то же относится к
системным библиотекам Windows. Поэтому «полностью статический» сказано **только про
linux**, а про darwin/windows — «без CGO, с обычной для платформы привязкой к системной
библиотеке, не проверено запуском».

### 1.2. Встроенная версия

`mcd version` печатает `report.EngineVersion`. Этот идентификатор объявлен **константой**
в `engine/report/report.go`, а `-ldflags -X` умеет писать только в строковую **переменную**,
и `-X` по константе молча не применяется. Каталог `engine/` на время G6 принадлежит
другому агенту (адденда G5), поэтому править `report.go` было нельзя.

Что `-X` по константе не срабатывает — замерено, а не предположено: сборка тем же
`-ldflags` без overlay завершается кодом 0 и ничего не сообщает, а бинарник печатает

```
$ CGO_ENABLED=0 go build -ldflags "-X modelcheck/report.EngineVersion=STAMP-TEST" -o mcd-noverlay ./cmd/mcd
$ ./mcd-noverlay version
mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)
```

то есть штамп не применился, и сборка прошла бы «успешно» с неверной версией.

Решение: `build.sh` собирает через `go build -overlay`, подставляя сгенерированную копию
`report.go`, в которой единственная строка `EngineVersion = "…"` вынесена из `const`-блока
в `var`. Ни один файл репозитория при этом не меняется, временная копия удаляется
в `trap`. Скрипт отказывается собирать, если форма объявления изменилась, и **проверяет
результат**: если `mcd version` не содержит запрошенную версию, сборка падает, а не
завершается успешно с неприменившимся штампом.

```
$ ./build.sh --version 0.1.0-g6-test
build.sh: mcd 0.1.0-g6-test (ir mcd-ir/1, report mcd-report/1)
```

Тот же штамп доходит и до MCP: `serverInfo` в §2 сообщает `{"name":"mcd","version":"0.1.0"}`
для релизной сборки.

Если константа когда-нибудь станет переменной, overlay пропускается сам
(`stamp_overlay` это проверяет). **Рекомендация владельцу:** заменить в
`engine/report/report.go` `const EngineVersion` на `var EngineVersion` — одна строка,
после которой overlay перестаёт быть нужен. Править сейчас нельзя было из-за границы
владения, в план это изменение не входит.

Единственный источник версии — `plugin.json`: `build.sh` без `--version` читает
`version` оттуда, так что `mcd version` и версия плагина не могут разойтись.

### 1.3. Воспроизводимость

`./build.sh --host-only --verify-repro` собирает второй раз во временный каталог и
сравнивает sha256 каждого файла:

```
build.sh: reproducible (two builds agree on every binary)
```

### 1.4. Как плагин находит бинарник

MCP-конфиг плагина называет **одну** команду, `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd`, и
эта строка не менялась (переехал только сам файл — §3). Изменилось то, чем является
`mcd`: теперь это POSIX-обёртка,
которая по `uname -s`/`uname -m` выбирает `mcd-<goos>-<goarch>` в именах
`runtime.GOOS`/`runtime.GOARCH`.

Почему обёртка, а не суффикс в манифесте: в справочнике манифеста Claude Code
(`code.claude.com/docs/en/plugins/manifest-reference.md`) для stdio-серверов перечислены
только подстановки `CLAUDE_PLUGIN_ROOT` и `CLAUDE_PLUGIN_DATA`; **выбор команды по
платформе не документирован**, и способа записать его в MCP-конфиге нет. Значит выбор
приходится делать ниже манифеста.

Обёртка намеренно не зависит от `dirname(1)`: первый прогон песочницы (§2) показал, что
команда плагина не должна предполагать на машине больше, чем POSIX-оболочку и `uname`,
поэтому каталог берётся как `${0%/*}`. `mcd.cmd` для Windows-оболочек в поставку кладётся,
но **здесь не проверялся** и помечен так и в скрипте, и в таблице §1.1.

Бинарники — артефакт релиза, а не содержимое репозитория: `.gitignore` исключает
`model-check-plugin/engine/bin/mcd-*` (44 МБ на пять платформ). В git лежат обёртки,
`SHA256SUMS` и `BUILD-INFO.json`, и последний называет коммит, из которого собран
описанный набор.

---

## 2. Проверка установки (A6): машина без Go

Песочница: копия `model-check-plugin/` во временном каталоге; `PATH` — приватный каталог
из **2 268** символьных ссылок на все обычные утилиты этой машины **кроме** `go`, `gofmt`,
`godoc`, `gccgo`, `tinygo`; из окружения убраны `GOROOT`, `GOTOOLCHAIN`, `GOPATH`,
`GOFLAGS`, `GOBIN`, `GOCACHE`, `GOMODCACHE`.

Первая попытка выбрасывала из `PATH` целые каталоги, где лежит `go`. Это неверно:
`/usr/bin` держит и `go`, и `uname`, так что такая песочница проверяла бы машину без
coreutils, а не машину без Go — и обёртка честно падала на `dirname: not found`. Исправлены
обе стороны: песочница (ссылки вместо выбрасывания каталогов) и обёртка (§1.4).

Сервер запускался **командой из MCP-конфига плагина** (то есть через обёртку), с
`${CLAUDE_PLUGIN_ROOT}`, указывающим в песочницу:

| проверка | результат |
|---|---|
| `go` на `PATH` песочницы | `None` |
| Go-исходников в песочнице | 92 `.go`-файла — лежат и **не компилируются** |
| `initialize` | `serverInfo {"name":"mcd","version":"0.1.0"}` |
| `tools/list` | ровно 7: `mc_check`, `mc_estimate`, `mc_explain`, `mc_lint_property`, `mc_manifest`, `mc_parse`, `mc_simulate` |
| `mc_parse` (petrinet1, inline JSON) | `outcome: "ir"`, сессия `s2026…-0001` |
| `mc_check` (`deadlock`) | `violated` / `exhaustive`, контрпример `t1, t4` |
| закрытие stdin | код выхода `0` |

Вердикт совпадает с `petrinet1` из G0. Сам по себе этот факт движок не доказывает —
заглушка, возвращающая заранее известный ответ, дала бы то же самое (утверждение
следствия). Основание, которое заглушке далось бы труднее: сервер завёл каталог сессии,
записал туда канонический IR и файл трассы, а `tools/list` вернул схемы входа и выхода
всех семи инструментов.

**Границы утверждения.** Песочница — копия каталога на этой же машине с вырезанным
Go-инструментарием, а не свежая операционная система. Она показывает: (1) **на этой платформе (linux/arm64)** готового бинарника достаточно и
компилятор не нужен; (2) команда из MCP-конфига плагина разрешается и запускается; (3) семь
инструментов и один содержательный round-trip работают. Она **не**
показывает поведение на машине без glibc нужной версии, на другой ОС и при установке из
маркетплейса. Слова «устанавливается на чистой машине» в §6 употребляются только в этом
объёме.

---

## 3. Двойная регистрация: измерено, найдена, исправлена

Вопрос из G2 §4 № 10: `plugin.json` указывает на `.mcp.json`, и `.mcp.json` в корне плагина
подхватывается сам — не регистрируется ли сервер дважды.

**Ответ: при прежней упаковке — да, при определённом условии. Условие найдено замером,
упаковка изменена, после изменения регистрация одна во всех проверенных случаях.**

### 3.1. Что говорит документация

`code.claude.com/docs/en/plugins/manifest-reference.md`, раздел `mcpServers`, дословно:
«Claude Code loads `.mcp.json` at the plugin root first, then each declared shape in order.
A server name declared later replaces an earlier one». То есть **в пределах плагина**
источника два, но позднее имя замещает раннее, и плагин регистрирует сервер один раз.

Этого правила, однако, недостаточно: оно говорит только о том, как плагин собирает свою
конфигурацию. Проектный `.mcp.json` — **отдельный путь регистрации** с собственной областью
и собственным подтверждением пользователя.

### 3.2. Что показал реальный клиент

`claude --plugin-dir <плагин> mcp list`, Claude Code 2.1.273, прежняя упаковка
(`.mcp.json` в корне плагина):

| рабочий каталог | что напечатал клиент |
|---|---|
| корень репозитория | одна запись: `plugin:model-check:model-check … ✔ Connected` |
| **сам каталог плагина** | **две записи**: `plugin:model-check:model-check … ✔ Connected` и `model-check: ${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd … ⏸ Pending approval`, плюс диагностика «Project config (shared via `.mcp.json`) … [Warning] mcpServers.model-check: Missing environment variables: CLAUDE_PLUGIN_ROOT» |

Вот та самая двойная регистрация, о которой спрашивает критерий выхода. Механизм: когда
каталог плагина **сам является рабочим каталогом**, его `.mcp.json` читается второй раз —
уже как **проектный** конфиг. Вторая запись не просто лишняя, она неработоспособна:
`${CLAUDE_PLUGIN_ROOT}` вне контекста плагина не подставляется, клиент честно пишет об этом
в диагностике и просит подтверждения на сервер, который не запустится.

Случай не экзотический: именно так открывает каталог всякий, кто разрабатывает сам плагин —
в том числе этот шаг.

### 3.3. Что изменено

Файл перенесён: `.mcp.json` → **`mcp/servers.json`**, `plugin.json` указывает на него
(`"mcpServers": "./mcp/servers.json"`). Содержимое файла не менялось ни на байт: команда
та же, потолки те же. В корне плагина `.mcp.json` больше нет, значит второй путь
регистрации исчез.

Контрольный замер после переноса:

| вариант | рабочий каталог | результат |
|---|---|---|
| прежний (`.mcp.json` в корне) | каталог плагина | **две** записи (одна Pending approval, сломанная) |
| прежний | корень репозитория | одна запись, Connected |
| **новый (`mcp/servers.json`)** | каталог плагина | **одна** запись, Connected |
| **новый** | корень репозитория | **одна** запись, Connected |

`claude plugin validate --strict` после переноса — `✔ Validation passed`, код выхода 0.
Ради него в `plugin.json` добавлено поле `author`: без него валидатор выдавал
предупреждение, а `--strict` считает предупреждение ошибкой.

Сценарий feature-файла закрепляет обе половины: объявленные источники дают ровно одно имя
сервера, **и** в корне плагина нет `.mcp.json`. Если файл вернут на место, сценарий падает
с текстом, называющим причину.

Цена решения названа честно: план 14 §3 рисует `.mcp.json` в корне плагина, и эта раскладка
изменена. Основанием считаю критерий выхода той же строки плана §9, который прямо требует
отсутствия двойной регистрации, — требование сильнее, чем схема каталога, и именно оно
здесь проверялось замером.

### 3.4. Чего здесь нет

Плагин **не устанавливался** через маркетплейс (`claude plugin marketplace add` +
`claude plugin install`): для этого нужен `.claude-plugin/marketplace.json`, которого план
не предусматривает. Проверен режим `--plugin-dir`, который документация называет загрузкой
плагина на сессию. Утверждение «реальный клиент подтверждает» относится именно к нему.
Равенство двух путей установки — посылка, не замер.

---

## 4. Триггер-точность описания

**Метод.** План §8.2 предполагал спрашивать свежий `claude -p`. В этом окружении headless
CLI отвечает `Not logged in · Please run /login` и ни одного запроса не выполняет, поэтому
измерение сделано **вторым разрешённым способом: судьями-субагентами**. Каждому судье
даётся ростер доступных скиллов, в котором `model-check` несёт проверяемое описание, плюс
десять скиллов-отвлечений с настоящими описаниями, и двадцать запросов; он отвечает
да/нет на «стал бы ты открывать `model-check` перед ответом». Метки судье не показываются,
порядок запросов один и тот же для всех вариантов и перемешан зерном, так что классы не
идут группами. Три прогона на вариант, решение по большинству; ничья считается «нет».

**Набор.** `evals-workspace/trigger-eval.json` — 20 запросов: 10 «должен» и 10
«не должен», русские и английские в обоих классах, у каждого записана причина, по которой
он отнесён к своему классу. Путь к файлу несут **8 из 10** триггерных запросов; два
(`t03` — сеть Петри, заданная словами, и `t08` — два сервиса, берущие блокировки в разном
порядке) стоят без пути намеренно: это ветка «система описана словами» (план §7.2 шаг 2 и
допущение A7), без которой набор проверял бы только узнавание расширения `.pml`.
Близкие промахи — из списка плана §8.2: юнит-тесты
параллельного кода, `go test -race`, доказательство одной функции в Coq, производительность
самого SPIN, рисование сети без анализа, общий вопрос «что такое model checking», а также
подсветка синтаксиса `.pml`, профилирование, рефакторинг автомата и перевод документации.

**Разбиение.** 60/40 со стратификацией по классу, зерно 20260926: обучающая часть 12
запросов, held-out 8 (4 и 4). Стратификация не украшение: нестратифицированное 60/40 на
двадцати элементах может оставить held-out одноклассовым, а точность на одном классе не
измеряет ничего. Порог объявлен заранее: **held-out ≥ 0,85**.

Оговорка, без которой число читается неверно: разбиение защищает от подгонки только тогда,
когда подгонка была. Итераций описания в этом прогоне **ноль** (почему — ниже), значит
подгонять было нечего, и «held-out 1,0000» здесь — просто точность на восьми запросах, а не
величина, защищённая разбиением. Разбиение сделано и закреплено зерном на будущее: как
только описание начнут править по этим же запросам, held-out заработает.

**Результат.**

| вариант | что это | train (12) | **held-out (8)** | согласие 3 прогонов |
|---|---|---:|---:|---:|
| `v0` | описание, которое `SKILL.md` несёт сегодня (действующее) | 1,0000 | **1,0000** | 1,00 |
| `vmin` | контроль: краткая однострочная формулировка из `plugin.json` | 1,0000 | 1,0000 | 1,00 |
| `vanti` | контроль: описание «скилл только форматирует Promela» | 0,5000 | 0,5000 | 1,00 |

`v0`: 10 TP, 10 TN, 0 FP, 0 FN. `vanti`: 0 TP, 10 FN, 0 FP, 10 TN — судья не открыл скилл
ни разу.

Точечная оценка — не интервальная. 8 из 8 на held-out совместимо с долей примерно от 0,63
(нижняя граница 95 % по Уилсону), 12 из 12 на обучающей доле — примерно от 0,74. Порог
0,85 пройден точечной оценкой; интервально набор из двадцати запросов подтвердить его не
может.

**До и после — и почему «после» равно «до».** Действующее описание набирает на held-out
1,0000, то есть потолок, и улучшать по этому измерению нечего: любая правка может только
не превзойти. Правило ничьей в `trigger_measure.py` зафиксировано заранее — при равном
held-out побеждает действующий вариант, и обучающая доля ничью не разрешает, потому что
именно её критерий выхода запрещает подставлять. Поэтому **описание `SKILL.md` не
изменено**, а «оптимизация описания» из строки G6 закрыта выводом «текущее описание уже на
потолке этого набора», а не правкой. Русские и английские триггер-фразы остались на месте
(их наличие проверяет отдельный сценарий feature-файла).

**Честная оговорка о самом измерении.** `vmin` — заведомо более бедное, но правдивое
описание — набирает столько же, сколько `v0`. Значит измерение **не различает** краткое
правдивое описание и подробное; различает оно правильное и неправильное (`vanti` рушится
до 0,5000). Число 1,0000 поэтому говорит «описание не мешает срабатыванию и не ловит
ложных», а не «это описание лучше другого».

Почему получился потолок — два объяснения, которые эти данные **не разделяют**:

1. **набор слишком лёгкий** — близкие промахи недостаточно близки, и запрос сам по себе
   выдаёт свой класс;
2. **метод судейства нечувствителен к формулировке** — сильная модель восстанавливает
   намерение из текста запроса, как только описание правдиво, и к различиям в подробности
   безразлична.

`vanti` показывает лишь, что судья читает описание, когда оно запросу **противоречит**; о
чувствительности к более тонким различиям он не говорит ничего. Эксперимент, который
разделил бы два объяснения: взять два правдивых описания, различающихся **только** наличием
русских триггер-фраз, и судить ими подмножество запросов только по-русски — если точность не
изменится, дело в методе, а не в наборе. Здесь он не поставлен.

Итераций описания сделано **ноль из трёх разрешённых** — не потому, что их не пытались
провести, а потому, что на этом наборе результат правки был бы неотличим от исходного, и
любая «победившая» формулировка победила бы по шуму. **Рекомендация к плану:** если
триггер-точность должна управлять формулировкой, нужно и то и другое — близкие промахи
существенно ближе (например «проверь, что мой парсер не зациклится на этом входе», «докажи,
что эта чистая функция всегда возвращает положительное» — вопросы о свойствах, но не о
параллельной системе) и различающий эксперимент выше; иначе метрика остаётся приёмочной, а
не оптимизационной.

---

## 5. Полный набор evals: итерация 4

Прогон: семь eval'ов × две конфигурации = 14 прогонов субагентами, каталог
`evals-workspace/iteration-4/`, движок — **упакованный** `engine/bin/mcd` 0.1.0 (обёртка →
`mcd-linux-arm64`), собранный уже **после** адденды G5 `e61f5da`, которая убрала stutter
extension из произведения с `np_`. Поэтому оговорка «E4 может сдвинуться, когда правка
придёт», предусмотренная постановкой шага, **не понадобилась**: правка пришла до прогона.
E6 исполним впервые — CTL появился в G5.

### 5.1. Результат по eval'ам

| eval | что проверяет | `runnable_from` | со скиллом | без скилла |
|---|---|---|---:|---:|
| E1 | `mutex_flaw`, нарушенный инвариант | G1 | **8/8** | 6/8 |
| E2 | alternating bit, liveness и справедливость | G4 | **8/9** | 5/9 |
| E2b | голодание, лассо, weak fairness | G4 | **9/9** | 8/9 |
| E3 | сеть Петри со слов → тупик `t1, t4` | G0 | **9/10** | 5/10 |
| E4 | телефон, `progress`/liveness | G4 | **9/9** | 3/9 |
| E5 | `c_code` вне подмножества → `not-executed` | G1 | **7/7** | 3/7 |
| E6 | `AG EF idle`, CTL без подмены на LTL | G5 | **7/7** | 5/7 |
| **итого** | | | **57/59 = 0,966** | **35/59 = 0,593** |

Все семь eval'ов с `runnable_from ≤ G5` имеют graded-прогон в обеих конфигурациях; это
проверяет отдельный сценарий feature-файла, а не только глаз.

### 5.2. Две непройденные assertion'а — и почему они не исправлены

Со скиллом не прошли ровно две проверки, **обе о форме ответа, ни одна о вердикте**:

1. **E2, «The engine was invoked (CLI or MCP), not reasoned around»** — регулярное
   выражение ищет `mcd check|mc_check` в тексте ответа. Прогон движок вызывал (в
   `outputs/mc-session-2026-09-27/` лежат двадцать артефактов, включая отчёты проверок), но
   в самом `answer.md` названа только команда `mcd parse --promela`. Assertion права
   буквально: по тексту ответа вызов проверки не показан.
2. **E3, «The engine CLI was run on the net … (`mcd check --petri` …)»** — прогон пошёл
   документированным маршрутом «`mcd parse --petri` → дописать свойства в IR →
   `mcd check --ir`» (`engine-tools.md` §4), поэтому строки `mcd check … --petri` в ответе
   нет. Assertion кодирует **один** из двух законных маршрутов.

Assertion'ы **не правились**. Итерация 3 уже дважды уточняла assertion после грейдинга
(`steps/g3-evals3-confirmation.md`, оговорка 3), и повторить это здесь значило бы
подогнать измерение под результат: любое «уточнение» после просмотра оценок поднимает
балл по построению. Обе непройденные проверки оставлены красными, а предложение по E3
вынесено в рекомендации (§7) как правка **плана и файла eval'ов**, а не как правка
задним числом.

### 5.3. Бенчмарк по итерациям

Средние по eval'ам считаются на разных наборах (итерация 3 — шесть eval'ов, итерация 4 —
семь), поэтому рядом дан и пересчёт на **общих шести**, где и промпты, и assertion'ы
совпадают дословно:

| | итерация 2 | итерация 3 | итерация 4 | итерация 4, общие 6 eval'ов | итерация 3, те же 6 |
|---|---:|---:|---:|---:|---:|
| со скиллом, доля пройденных | — | 1,0000 | 0,9698 | **50/52 = 0,962** | **52/52 = 1,000** |
| без скилла | — | 0,4209 | 0,5958 | **30/52 = 0,577** | **22/52 = 0,423** |
| разрыв | — | +0,58 | +0,37 | +0,385 | +0,577 |

Как это читать. Разрыв сократился с двух сторон: со скиллом −2 проверки (§5.2), без скилла
+8 проверок на тех же 52. **Одного прогона на конфигурацию недостаточно, чтобы назвать это
изменением качества**: между итерациями менялся день, модель-исполнитель бралась та же, но
выборка одна, и сдвиг такой величины одна выборка даёт сама по себе. Ни «скилл стал хуже»,
ни «базовый агент стал лучше» из этих чисел не следует; следует, что разрыв со скиллом и без
него сохраняется и остаётся большим (+0,385 на общих eval'ах).

Что действительно видно без статистики: базовый агент во всех семи прогонах пользовался
установленным SPIN, а в E6 написал собственный CTL-чекер на Python с вычислением
неподвижных точек. Разрыв поэтому измеряет **контракт** скилла (статус из словаря,
evidence, разделение prefix/loop, запрещённые формулировки, честный `not-executed`), а не
способность найти правильный ответ — тот же вывод, что в итерации 3.

### 5.4. Гигиена прогона

Одиннадцать из четырнадцати прогонов — повторные: первую попытку оборвал лимит сессии.
Артефакты оборванных попыток (84 файла в семи каталогах `with_skill/outputs/`) удалены
**до** грейдинга и переписаны поимённо в `eval_metadata.json` каждого eval'а
(`removed_before_grading`). Удалены до, а не после, намеренно: `grader.py` кроме текста
ответа просматривает каталог выходов (`petri_json_valid`, `json_field`), и файл от
оборванной попытки мог бы закрыть проверку, которую зачтённый прогон не закрывал. Три
прогона `without_skill` (E3, E5, E2b) успели завершиться до лимита — они зачтены как есть,
и это в метаданных сказано.

Отдельно, уже **после** грейдинга, удалены пять файлов — дословные копии файлов корпуса
(`CH2/mutex_flaw.pml`, `CH2/peterson.pml`, `CH2/mutex.pml`, `CH3/alternatingbit.pml`,
`CH14/version1`), которые сделали три прогона `without_skill`: лицензионная заметка плана
§2.1 велит ссылаться на корпус путём и хешем, а не копировать его в поставку. Все пять
перечислены поимённо в `removed_after_run` соответствующих `eval_metadata.json`. На оценки
это повлиять не могло: грейдер читает `answer.md` и `*.json`, а удалены `.pml`. Затронуты
только базовые прогоны — промпт `with_skill` запрещал копирование явно, а промпт базового
прогона намеренно оставлен слово в слово таким же, как в итерациях 1–3, иначе конфигурации
перестали бы быть сравнимыми. Проверка после удаления: дословных копий корпуса в
`iteration-4/` — ноль.

`review.html` — 14 прогонов, у 12 из них показан прогон итерации 3 и его оценка
(E6 предшественника не имеет): `evals-workspace/iteration-4/review.html`, собран
`generate_review.py iteration-4 --previous-workspace iteration-3`.

---

## 6. Соответствие критерию выхода 14 §9 (строка G6)

| часть критерия | свидетельство | вердикт |
|---|---|---|
| **Все evals** | 7 eval'ов × 2 конфигурации, все graded, `iteration-4/benchmark.json`; со скиллом 57/59, без скилла 35/59; E6 исполнен впервые | **выполнено с оговоркой**: один прогон на конфигурацию — свидетельство, не статистика; две assertion'а красные (§5.2) и намеренно не подогнаны |
| **Триггер-точность на held-out ≥ порога** | порог 0,85 объявлен до замера; held-out 1,0000 (8/8), train 1,0000 (12/12), согласие трёх прогонов 1,00 | **выполнено точечной оценкой**; содержательно ограничено: интервально набор порог не подтверждает, измерение насыщено (`vmin` = `v0`), а split в этом прогоне ничего не защитил, потому что итераций было ноль |
| **Плагин устанавливается на чистой машине без Go** | песочница без Go-инструментария: сервер стартует из упакованного бинарника, 7 инструментов, `mc_check` → `violated`/`exhaustive`, `t1, t4`, выход 0; `claude plugin validate --strict` — passed | **выполнено частично**: показано на linux/arm64 и на копии каталога; другие четыре платформы собраны и захешированы, но не запущены; установка из маркетплейса не проверялась |
| **Реальный клиент подтверждает, что `plugin.json` и корневой `.mcp.json` не регистрируют сервер дважды** | замером найдено, что прежняя упаковка регистрировала сервер **дважды**, когда рабочим каталогом был сам каталог плагина (вторая запись — проектная, Pending approval, с неподставленным `${CLAUDE_PLUGIN_ROOT}`); конфиг перенесён в `mcp/servers.json`, после чего `claude mcp list` даёт одну запись `✔ Connected` из обоих рабочих каталогов; `plugin validate --strict` — passed | **выполнено**: дефект найден и устранён, а не объявлен отсутствующим |

**Итог: критерий выхода выполнен частично.** Невыполненного нет; ограничения — в объёме
проверки (одна платформа из пяти запущена, один прогон на конфигурацию, насыщенная
триггер-метрика), а не в отсутствии результата.

---

## 7. Тесты, файлы, отложенное

### 7.1. Тесты

```
godog (общий харнесс, -count=1, на HEAD после G3/G5):
  334 scenarios (334 passed); 1832 steps (1832 passed)   — go test: ok, 107 s
  — features/g6-package.feature: 10 сценариев, все зелёные
go vet ./...                    — чисто;  gofmt -l steps_g6_test.go — пусто
python3 -m unittest (evals-workspace): Ran 39 tests — OK
  (9 grader + 9 aggregate + 21 trigger_measure)
```

Общее число сценариев выросло с 315 до 334 в ходе шага: параллельные агенты G3 и G5
добавляли свои. Красных нет ни одного.

Юнит-тесты добавлены на весь новый вспомогательный код (протокол, п. 3):
`test_trigger_measure.py` — 21 тест на загрузку набора (отказ на дубликат запроса и на
нелогическую метку), на разбиение (детерминированность по зерну, разбиение без пересечения,
оба класса с обеих сторон, размеры 12/8), на большинство голосов (два из трёх, ничья = «нет»,
пропуск не голосует), на подсчёт (матрица ошибок, подсчёт только запрошенных id, пропуск
назван а не зачтён), на промпт (метки скрыты, все запросы на месте, порядок одинаков для
вариантов и перемешан, ростер несёт отвлечения) и на правило выбора (побеждает лучший
held-out; ничья остаётся за действующим; лучший train не побеждает).

### 7.2. Файлы

Созданы: `build.sh`; `engine/bin/{mcd, mcd.cmd, SHA256SUMS, BUILD-INFO.json}`;
`features/g6-package.feature`; `engine/steps_g6_test.go`;
`evals-workspace/{trigger-eval.json, trigger_measure.py, test_trigger_measure.py,
trigger-results.json, trigger-runs/}`; `evals-workspace/iteration-4/` (7 каталогов,
14 прогонов, `benchmark.json`, `benchmark.md`, `review.html`);
`steps/{g6-logika.md, g6-confirmation.md}`.

Изменены: `.claude-plugin/plugin.json` (добавлено `author` ради `--strict`; `mcpServers`
теперь указывает на `./mcp/servers.json`); `.gitignore` (исключены `engine/bin/mcd-*`).
Перемещён: `.mcp.json` → `mcp/servers.json` (§3; содержимое побайтово то же).

**Не тронуты:** `engine/` кроме `steps_g6_test.go` (каталог принадлежал агенту G5);
`skills/` целиком, включая `SKILL.md` — описание менять не пришлось (§4), а тело
принадлежит агенту G3; `Promela - examples/`; `model-check-skill-notes/`.

Зависимостей не добавлено — ни Go, ни Python: `trigger_measure.py` и харнесс песочницы
стоят на стандартной библиотеке, `steps_g6_test.go` говорит с сервером сырым JSON-RPC
поверх stdin/stdout, а не через SDK, потому что проверяется именно запуск упакованного
бинарника так, как его запускает клиент.

### 7.3. Отложено и почему

1. **Запуск на darwin и windows** — нет машин; бинарники собраны и захешированы, `mcd.cmd`
   не проверялся ни разу.
2. **Установка из маркетплейса** — нужен `.claude-plugin/marketplace.json`, которого план
   не предусматривает; проверен режим `--plugin-dir`.
3. **Вызов `mc_*` самим скиллом в установленном плагине** — остаётся невыполненным с G2.
   Субагент не может зарегистрировать MCP-сервер в свою сессию, поэтому все 14 прогонов шли
   по CLI. Установлено взамен: сервер регистрируется ровно один раз и проходит health-check
   реального клиента (§3), а упакованный сервер отвечает на семь инструментов и один
   round-trip по stdio (§2). Связка «скилл → MCP» целиком проверена не была.
4. **Интервальное подтверждение триггер-порога** — нужен набор существенно больше двадцати
   запросов (§4).
5. **Повторные прогоны eval'ов** — по одному на конфигурацию; для статистики нужно
   несколько прогонов на ячейку.

### 7.4. Рекомендации, требующие правки плана 14 (сам план не правил)

1. **§9, строка G6.** Записать, что «оптимизация описания» может закончиться выводом «набор
   насыщен», и что порог сравнивается с held-out, а train приводится рядом. Сегодня строка
   предполагает, что итерации описания обязательно будут.
2. **§8.2, триггер-набор.** Близкие промахи в списке плана слишком далеки от триггеров:
   измерение различает правильное описание и неправильное, но не две правдивые
   формулировки. Нужны промахи вида «вопрос о свойстве, но не о параллельной системе»
   (§4), иначе метрика приёмочная, а не оптимизационная.
3. **Assertion E3 в `evals/evals.json`** кодирует один из двух законных маршрутов
   (`mcd check --petri` против `mcd parse --petri` → `mcd check --ir`). Предлагается
   допустить оба — но **правкой плана и файла eval'ов отдельным решением**, не задним
   числом в этом шаге: после просмотра оценок такая правка поднимает балл по построению.
4. **`engine/report/report.go`: `const EngineVersion` → `var EngineVersion`** — одна строка,
   после которой `build.sh` перестаёт нуждаться в overlay (§1.2). Принадлежит владельцу
   движка.
5. **`SKILL.md`, раздел «чего скилл не делает»**, всё ещё говорит, что CTL не проверяется
   (`ctl` → `not-executed`, G5), тогда как шаг 4 того же файла, `engine-tools.md` §8 и
   `properties-ltl-ctl.md` §2 говорят обратное, и прогон E6 это подтверждает. Найдено
   прогоном E6; **не исправлено здесь**, потому что тело `SKILL.md` принадлежит агенту G3.
6. **§3 плана (раскладка поставки) больше не совпадает с фактом:** MCP-конфиг лежит не в
   `.mcp.json` корня плагина, а в `mcp/servers.json`. Причина — измеренная двойная
   регистрация (§3 подтверждения); менять надо схему в плане, а не возвращать файл.
7. **`references/engine-tools.md` строка «The plugin's `.mcp.json` starts it as …»**
   называет файл прежним именем. Команда в ней верна, устарело только имя файла.
   **Не исправлено здесь**, потому что `skills/` принадлежит агенту G3; предлагаемая
   формулировка — «The plugin's MCP config (`mcp/servers.json`, named by `plugin.json`)».
   Сценарий G3 `features/g3-evals.feature` проверяет, что в справке **упоминается** строка
   `.mcp.json`, и от переноса файла он не краснеет — проверено полным прогоном.
8. **A6 (§12) выполнено в объёме одной платформы.** Стоит записать в план, что «чистая
   машина» для A6 означает «без Go-инструментария», и что проверка запуском на пяти
   платформах требует пяти машин или CI-матрицы, которой в плане нет.

## 7.5. Codex and Coddy integration addendum

Scope: portable agent-facing files added to the standalone plugin checkout:
`AGENTS.md`, `.codex/README.md`, `.coddy/mcp.json`, and `.coddy/README.md`, plus
the G6 packaging scenario that checks their presence and the portable Coddy command.

Verification on 2026-09-30:

- `claude plugin validate model-check-plugin` — passed.
- `go test -count=1 -run TestFeatures` — **396 scenarios and 2040 steps passed**.
- `go test ./...` — passed, including `modelcheck/tools/pandiff`.
- `go vet ./...` — passed.
- `go fmt ./...` and `git diff --check` — passed.
- Codex CLI `codex-cli 0.159.2`, invoked through `npx --yes @openai/codex exec`
  in ephemeral/read-only mode, read the new files and ran
  `engine/bin/mcd version`: `mcd 0.1.0 (ir mcd-ir/1, report mcd-report/1)`.
- The Codex command surface was exercised in a temporary `CODEX_HOME`: `codex mcp
  add`, `codex mcp get`, and `codex mcp list` recorded exactly one `model-check`
  stdio server with the expected command and limits. The temporary configuration
  was removed afterwards.
- Coddy 1.2.45 ran the same read-only one-shot prompt successfully and reported
  the same engine version. `coddy mcp list --cwd model-check-plugin` showed the
  project-local `model-check` server as ready with `${CWD}/engine/bin/mcd`.

The Coddy global `--dry-run` was also attempted. It reported one timeout from the
unrelated global `whentofly` server and missing optional global directories; this
does not invalidate the local `model-check` declaration, which was independently
listed as ready. No user files, credentials, or permanent Codex configuration were
changed by these checks. The addendum demonstrates discovery, command resolution,
and CLI execution; it does not claim that every global Coddy MCP server is healthy.
