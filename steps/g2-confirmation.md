# G2 — подтверждение (протокол, п. 6)

Шаг: **G2** (план 14 §9, строка G2). Критерий выхода: «Skill вызывает все инструменты; лимиты срабатывают; работа вне каталога невозможна».

**Соответствие критерию выхода: выполнен** — с двумя оговорками, записанными в §6: (1) «skill вызывает» подтверждено клиентом MCP из SDK (in-memory в тестах, stdio вручную), а не самим skill'ом в Claude Code — установка плагина и вызов из skill'а по плану принадлежат G6; (2) «невозможна» держится на проговорённой посылке, что в каталог сессии пишет только сервер (TOCTOU, ревью № 10).

## 1. Что сделано

- `features/g2-mcp.feature` — написан до кода (коммит `G2: feature file … (before code)`): 29 сценариев + outline с 3 примерами = 31 строка. Шапка фиксирует три рода ответов (отказ инструмента / отвержение входа / результат проверки — NFR-007), словарь статусов и evidence с отсылкой к смыслам G0, правило агрегации, устройство каталога сессии и бюджетов. Сценарии: ровно семь инструментов с объектными схемами входа и выхода; `mc_parse` petrinet1 → IR, session id, файл IR в каталоге сессии (равен `mcd parse` с точностью до поля origin file), таблица origins; отвержение inhibitor-дуги как структурный ответ, а не ошибка; Promela → `not-executed` с названием фронтенда; чтение файла только под `--allow-read`; `mc_check` petrinet1 → `deadlock violated/exhaustive` с контрпримером `t1, t4` в каталоге сессии и `safe verified`; крошечный бюджет states/depth → `inconclusive/bounded`, `complete=false`, ресурс назван; бюджет выше потолка → обрезан, `budget_notes` называют поле, запрос и потолок; частичный бюджет (одно поле) принимается; нулевой бюджет → умолчания в пределах потолка; ltl/progress/ctl → `not-executed/unknown` с G4/G5 и названием возможности; неизвестный kind → ошибка инструмента; нарушенный инвариант → `violated` и `mc_explain` с диффами, именами пользователя, пустым loop и заметкой про G4; агрегат только по запросу и по приоритету; `mc_check` без входа → ошибка; необъявленная переменная в свойстве → `rejected`; два одинаковых `mc_check` без таймингов → байт-в-байт равные файлы отчётов; id контрпримера `../../escape` → отказ с упоминанием каталога сессии и без файла; писатель сессии отвергает `../`, `sub/../../`, абсолютный путь и symlink наружу; `mc_simulate` random воспроизводим по seed, guided останавливается на неразрешённом ребре с перечнем разрешённых, тупик под управлением называется тупиком; `mc_lint_property` — атомы, неопределённые атомы, класс, X-free, константа как кандидат на vacuity; `mc_estimate` — состояния, скорость, таблица уровней, rate, evidence `approximate`, «не результат проверки»; `mc_manifest` — движок, схемы, входы с sha256, вызовы по порядку с длительностью и исходом, применённый бюджет; неизвестная сессия → ошибка.
- `engine/mcp/` (пакет `mcp`, ≈2 000 строк с тестами):
  - `server.go` — `Config`, `New`, регистрация семи инструментов через `mcp.AddTool[In, Out]` SDK с типизированными обработчиками `ToolHandlerFor`; `Run` (stdio), `Connect` (любой транспорт; тесты — in-memory); семафор `acquire`; `Rejection`; `rejectedInput`; `modelFor`/`adopt` (inline IR или модель сессии; канонический IR пишется в `ir-N.json`, вход с sha256 — в manifest); `allowedRead` (префиксы `--allow-read`, после `EvalSymlinks`).
  - `guard.go` — `Resolve(dir, rel)`: относительное имя → путь внутри каталога сессии; лексическая очистка, `EvalSymlinks` самого длинного существующего префикса, `filepath.Rel` к разрешённому каталогу; отказ на пустое, абсолютное, `..`, symlink наружу. Единственное место, где образуется путь к файлу сессии.
  - `session.go` — `Sessions` (базовый каталог, `Mkdir` с проверкой коллизий, opt-in `Cleanup`), `Session` (`WriteFile` — единственный писатель, `ReadFile`, `Path`, `next` для имён `check-N.json`/`sim-N.json`/`cex/cex-N.json`), `Manifest` (движок и схемы `mcd-ir/1`, `mcd-report/1`, `mcd-mcp/1`; параметры сервера; входы с sha256; вызовы с временем, длительностью, исходом `ok|error`, параметрами — search/fairness/budget_applied/seed/steps/mode/time_limit — и артефактами); `manifest.json` перезаписывается после каждого вызова.
  - `budget.go` — `Budget` (четыре необязательных поля), `DefaultBudget` = умолчания CLI (A4), `Clamp` (0 → умолчание; выше потолка → потолок и заметка), `within` (умолчания вписываются в потолки при старте).
  - `parse.go` — `mc_parse`: ровно один из `promela|petri|ir|file`; политика чтения до создания сессии; `outcome ∈ {ir, rejected, not-executed}`; таблица origins по элементам IR; Promela → `not-executed` с причиной «promela frontend not available in this build (engine/frontend/promela, step G1 …)» при `Config.Promela == nil`.
  - `check.go` — `mc_check`: валидация свойств (шесть kinds плана, `expr` как IR-выражение или имя переменной) до всякой работы с сессией; замена свойств модели; предупреждение о `fairness weak`; `Clamp`; семафор; deadline контекста из `ms`; `explore.Run` → `report.Build`; для ltl/progress/ctl `reason` переписывается на текст, называющий возможность и шаг (G4/G5); отчёт в `check-N.json`; трассы в `cex/cex-N.json` со ссылками `{id, path, summary, steps, user_names}`; агрегат по запросу. `mc_explain`: guard над id раньше поиска; prefix с диффами и именами пользователя, `loop` пуст с заметкой про G4. `mc_manifest`.
  - `simulate.go` — `mc_simulate`: random (`math/rand/v2` PCG от seed — воспроизводимо) и guided (id `process/index`, текст ребра или origin-имя); семичленный `stopped`; трасса `cex.Build` в `sim-N.json`, inline при ≤ 50 шагов; `enabled_at_stop`.
  - `lint.go` — `mc_lint_property`: атомы по первому вхождению, неопределённые (не глобальные), `ir.Check`, класс `safety` (invariant) / `reachability` (reach) с основанием, `x_free=true`, `temporal=false`, `constant` как кандидат на vacuity; kinds ltl/ctl/progress отвергаются ошибкой с названием шагов G4/G5.
  - `estimate.go` — `mc_estimate`: BFS-прогоны с растущим бюджетом глубины в пределах лимита времени; уровни (состояния на расстоянии ≤ d), геометрическое среднее последних отношений, проекция на следующий уровень с evidence `approximate` (или `exhaustive`, если граф раскрыт полностью); states/memory — умолчания сервера.
  - Юнит-тесты: `TestResolve` (13 форм путей, включая symlink наружу и внутрь), `TestWriteFileRefusesEscape`, `TestSessionsUniqueAndCleanup`, `TestClamp`, `TestWithin`, `TestSchemas` (семь имён, объектные схемы, тег → description, IR не превращается в массив байт), `TestToolErrorIsNotAResult`, `TestConcurrencySemaphore`, `TestAllowRead`.
- `engine/explore/step.go` — `Stepper` поверх приватных `compile`/`nextEnabled`/`fire` explorer'а (тот же порядок ходов, та же семантика rendezvous/d_step/доменов): `Enabled`, `Apply` (возвращает преемника и ребро с нарушенным assert), `Terminated`, `Move` с партнёром → `cex.Ref`. Адаптирован к параллельным правкам G1 в `explore.go` (тип `move`, `fire(move) (failed, err)`).
- `engine/cmd/mcd/serve.go` — `mcd serve [--session-dir D] [--allow-read DIR]... [--max-states N] [--max-depth N] [--max-ms N] [--max-memory-mb N] [--concurrency K] [--cleanup]`; `--session-dir` по умолчанию `$MCD_SESSION_DIR`, затем временный каталог; `dispatch` направляет `serve` в сервер, остальное — в `cli.Run`. `main.go` изменён в одной строке: `os.Exit(dispatch(os.Args[1:], cli.Run))`.
- `engine/steps_g2_test.go` — step definitions через `stepRegistrars`; сервер поднимается лениво (Given-шаги успевают поправить конфигурацию), клиент `sdk.NewClient` через `sdk.NewInMemoryTransports`; ответы читаются из `StructuredContent`, ошибки — из `IsError`/`Content`.
- `.mcp.json` — stdio-сервер `model-check`: `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd serve` с потолками (5e6 состояний, 5e6 глубина, 300 с, 2 ГиБ, concurrency 2). `.claude-plugin/plugin.json` — `name: model-check`, `skills: ./skills`, `mcpServers: ./.mcp.json`. Оба файла — **к валидации при установке (G6)**, см. §4.
- `steps/g2-logika.md` — ревью: 14 находок (4 критичных), 13 исправлены, 1 омонимия проговорена.
- Зависимость: `github.com/modelcontextprotocol/go-sdk v1.8.0` (прямая; `go.sum`: `h1:KIvahhYqwtbeniWVPs3TcXEA7b8jEtwfBpOTAI+Urx4=`), транзитивно `github.com/google/jsonschema-go v0.4.3` и др. (indirect). Собирается с Go 1.26.0 без правок. Использованный API SDK: `mcp.NewServer(&mcp.Implementation{…}, &mcp.ServerOptions{Instructions})`, `mcp.AddTool[In, Out](server, &mcp.Tool{Name, Description}, mcp.ToolHandlerFor[In, Out])` (схемы входа/выхода выводятся из Go-структур; описания полей — из тегов `jsonschema:"…"`; `any` даёт открытую схему, что нужно для рекурсивного `ir.Expr`, который jsonschema-go выводить отказывается — «cycle detected»; `json.RawMessage` выводится как массив байт и потому не используется), `Server.Run(ctx, &mcp.StdioTransport{})`, `Server.Connect(ctx, transport, nil)`, `mcp.NewInMemoryTransports()`, `mcp.NewClient(…).Connect`, `ClientSession.ListTools/CallTool`, `CallToolResult.IsError/StructuredContent/Content`, `*mcp.TextContent`. Ошибка Go из обработчика → `isError`-результат (SDK `server.go` v1.8.0, строки 421–433).
- Не тронуты: `skills/`, `features/g1-*`, `engine/frontend/promela`, `engine/cli`, `engine/steps_g1_test.go`, `engine/steps_g3_test.go`, `engine/internal/spike`, `engine/skillcheck`, `explore/explore.go` и другие файлы G0/G1.

## 2. Тесты

```
go vet ./mcp ./explore ./cmd/...   — чисто; gofmt — чисто (mcp, explore, cmd, steps_g2_test.go)
go test ./mcp ./explore            — ok (mcp: 9 юнит-тестов G2; explore: тесты G0 + step.go компилируется и используется ими)
godog (общий harness, -count=1):   142 scenarios: 89 passed, 1 failed, 52 undefined; 804 steps: 487 passed, 1 failed, 316 undefined
  — g2-mcp.feature: 31 строка сценариев (29 + outline 3), все PASS, ни одного красного шага;
  — 1 failed: g0-engine.feature, golden petrinet2 — отчёт теперь содержит поле `warnings`, добавленное
    незакоммиченными правками G1 в engine/report (не G2; golden-файл принадлежит G0/G1);
  — 52 undefined: features/g1-*.feature без step definitions (G1 в работе).
go build ./...                     — падает в engine/frontend/promela (незакоммиченный WIP G1: «Ident redeclared»);
                                     все пакеты G2 и cmd/mcd собираются (go build ./cmd/mcd — ok).
```

Ключевые проверки: `TestResolve` (symlink наружу отвергается, symlink внутрь допускается, `sub/../link-out/x` отвергается после лексической очистки); `TestWriteFileRefusesEscape` (после четырёх попыток под базовым каталогом нет файла `escape.json`); `TestSchemas`; `TestToolErrorIsNotAResult`; сценарии «two identical mc_check calls … byte-identical», «a client budget above the server ceiling is clamped», «a partial budget is accepted».

## 3. Замеры

Машина: linux/arm64, Go 1.26.0; та же сессия, что у параллельно работающих агентов G1/G3.

Ручной прогон бинарника `mcd serve --session-dir … --max-states 1000 --max-ms 5000` по **stdio** (JSON-RPC построчно, скрипт на Python; протокол `2025-06-18`):

| Шаг | Результат | Время |
|---|---|---|
| `initialize` | `serverInfo {mcd, 0.1.0-g0}` | — |
| `tools/list` | 7 инструментов, ответ 27 139 байт (схемы входа и выхода) | — |
| `mc_parse` petrinet1 inline | `outcome: ir`, session `s20260924-100214-0001` | 3,6 мс |
| `mc_check` с бюджетом `{states: 1000000}` (одно поле) | `deadlock violated/exhaustive`, `safe verified/exhaustive`; заметка «states: requested 1000000 exceeds the server ceiling 1000; clamped to 1000» | 0,9 мс |
| `mc_check` с kind `foo` | `isError: true`, текст «properties[0] (q): kind must be one of …»; в manifest не попадает (отказ до работы с сессией) | — |
| `mc_simulate` guided `t1, t4` | summary `t1, t4`, `stopped: deadlock` | — |
| `mc_estimate` 500 мс | 6 состояний, `complete: true`, projection evidence `exhaustive` | — |
| `mc_manifest` | вызовы `mc_parse, mc_check, mc_simulate, mc_estimate, mc_manifest`, все `ok` | — |
| вся сессия (8 запросов) | код выхода 0 после закрытия stdin | 20 мс |

Каталог сессии после прогона: `cex/`, `check-1.json`, `ir-1.json`, `manifest.json`, `sim-1.json` — и ничего вне него. Бинарник `mcd` — 11,3 МБ (SDK добавляет ≈ 7 МБ к бинарнику G0).

Этот прогон дал находку № 11 ревью (обязательность полей бюджета в схеме), которую 30 сценариев на in-memory транспорте не обнаружили — потому что всегда посылали четыре поля. Вывод для G6: проверять плагин реальным клиентом, а не только SDK-клиентом в процессе.

Накладные расходы сервера относительно CLI на petrinet1 не измеримы (миллисекунды); на крупных моделях доминирует движок, а сервер добавляет одну сериализацию отчёта и трасс в файлы.

## 4. Решения, зафиксированные в коде и требующие внимания владельца

1. **Три рода ответов** (NFR-007): ошибка инструмента = Go-ошибка → `isError` без структурного содержимого; отвержение входа = `outcome: rejected` + `rejection{kind, construct, file, line, reason}` у `mc_parse`/`mc_check`, у трёх инструментов без документа результата — `isError` с префиксом `rejected input (<kind>):`; результат проверки = структурный ответ, `isError=false` при любом статусе. Skill (G3/G6) должен читать класс по этим признакам.
2. **`0` в бюджете MCP = умолчание сервера**, не «без лимита» (в CLI 0 = unlimited). Клиент не может снять лимит, только запросить в пределах потолка. Все поля бюджета необязательны.
3. **Умолчания вписываются в потолки при старте** (`within`), поэтому заметки `budget_notes` появляются только для явных запросов клиента.
4. **Свойства запроса заменяют свойства модели**, но неявное `assert` движка добавляется в обоих случаях (решение G0 № 5). Пустой список свойств — ошибка инструмента, а не результат без свойств.
5. **`not-executed` в `mc_parse`** — исход разбора (фронтенд не подключён), омоним статуса проверки; задан постановкой шага, проговорён в feature и схеме.
6. **`mc_estimate` реализован в G2** как ограниченный BFS по уровням, хотя план относит `mc_estimate` к G5 (и G0 отложил «оценку роста» на G5). Оценка честная и помечена `approximate`; более точная модель роста (G5) может заменить внутренности без смены интерфейса. Рекомендация: в плане 14 §9 отметить, что интерфейс `mc_estimate` появился в G2, а содержательная оценка — G5.
7. **Каталог сессии**: `<base>/<id>`, `id = s<UTC время>-<счётчик>`, коллизии исключены `Mkdir`; cleanup opt-in (`--cleanup`); большие результаты — файлы, ответы несут абсолютные пути (только как выход; guard принимает лишь относительные имена).
8. **Промела**: `Config.Promela` — точка подключения фронтенда G1 (`func(src, defines, file) (*ir.Model, *Rejection, error)`); в этой сборке `nil`, и `mc_parse` отвечает `not-executed` с причиной, называющей фронтенд и шаг. Когда G1 закоммитит фронтенд и его вызов в `cli`, подключение — одна строка в `serve.go` (передать адаптер в `mcp.Config`), сценарий «Promela → not-executed» в feature надо заменить на позитивный.
9. **`.mcp.json` ссылается на `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd`** — бинарник надо собрать при установке (`go build -o engine/bin/mcd ./engine/cmd/mcd`); документация Claude Code не даёт декларативного шага сборки в `plugin.json` (см. §5), поэтому это работа G6 (SessionStart-hook или предсобранный бинарник). Потолки в `.mcp.json` (5e6/5e6/300 с/2 ГиБ) — предложение, а не измеренная граница.
10. **`plugin.json` одновременно указывает `mcpServers: ./.mcp.json`**, а корневой `.mcp.json` по документации подхватывается и сам; при установке (G6) проверить, что сервер не регистрируется дважды; если да — убрать одно из двух.

## 5. Источники по манифесту плагина (что проверено, что нет)

По документации Claude Code (`code.claude.com/docs/en/plugins-reference`, `…/docs/en/mcp`), прочитанной агентом-справочником в этой сессии: `plugin.json` — `name` обязателен, kebab-case; `skills` — строка или массив путей, добавляется к автоматически сканируемому `skills/`; `mcpServers` — путь к `.mcp.json` или inline-объект; переменные `${CLAUDE_PLUGIN_ROOT}`, `${CLAUDE_PLUGIN_DATA}`, `${CLAUDE_PROJECT_DIR}` допустимы в путях; запись stdio-сервера — `{type: "stdio", command, args?, env?}`; `.mcp.json` в корне плагина запускается при включении плагина; декларативного шага сборки нет (варианты — hook `SessionStart` или сборка внутри `command`). Схема не валидировалась установкой — **к валидации в G6**.

## 6. Соответствие критерию выхода 14 §9 (строка G2)

| Часть критерия | Свидетельство | Статус |
|---|---|---|
| Skill вызывает все инструменты | ровно семь инструментов в `tools/list` (сценарий + `TestSchemas`), каждый вызван хотя бы одним сценарием через клиент MCP из SDK и по stdio вручную (§3); схемы входа/выхода — объекты с описаниями полей | выполнено — через клиент SDK и stdio; вызов из установленного плагина — G6 |
| Лимиты срабатывают | сценарии states/depth → `inconclusive` с ресурсом и `complete=false`; потолок сервера обрезает запрос с заметкой (сценарий + stdio-прогон); умолчания при 0 и при отсутствии поля; deadline контекста из `ms`; семафор (`TestConcurrencySemaphore`); чтение файлов только под `--allow-read` (сценарии + `TestAllowRead`) | выполнено |
| Работа вне каталога невозможна | один писатель `Session.WriteFile` (единственный `os.WriteFile` в пакете — проверено grep) → `Resolve`; `TestResolve` 13 форм, `TestWriteFileRefusesEscape`, три сценария guard (лексический выход, абсолютный путь, symlink наружу) и сценарий с id контрпримера `../../escape` | выполнено при посылке «в каталог сессии пишет только сервер» (TOCTOU, ревью № 10) |

Дополнительно из содержания строки G2: manifest (FR-012, NFR-002) — версии движка и схем, параметры сервера, sha256 входов, seed, тайминги, лог вызовов; каталог сессии — есть.

## 7. Отложено и почему

- Вызов инструментов самим skill'ом в установленном плагине и валидация `plugin.json`/`.mcp.json` — G6 (нужна установка на чистой машине).
- Promela во `mc_parse` — G1 (фронтенд не закоммичен; точка подключения готова).
- Циклические контрпримеры (`loop` непустой), fairness, ltl/progress — G4; ctl — G5; lint временных формул — G4/G5.
- Содержательная оценка размера пространства состояний (`mc_estimate` G5) — интерфейс и ограниченная реализация есть, модель роста упрощённая.
- Автоматический тест stdio-транспорта в `go test` — не добавлен: он потребовал бы собрать бинарник внутри теста; сценарии идут через in-memory транспорт того же `mcp.Server`, а stdio проверен вручную (§3). Рекомендация G6: включить stdio-прогон в проверку установки.

## 8. Файлы

- `model-check-plugin/features/g2-mcp.feature`
- `model-check-plugin/engine/mcp/{server,session,guard,budget,parse,check,simulate,lint,estimate}.go`, `engine/mcp/{guard,budget,schema}_test.go`
- `model-check-plugin/engine/explore/step.go`
- `model-check-plugin/engine/cmd/mcd/serve.go`, `engine/cmd/mcd/main.go` (одна строка)
- `model-check-plugin/engine/steps_g2_test.go`
- `model-check-plugin/engine/go.mod`, `engine/go.sum`
- `model-check-plugin/.mcp.json`, `model-check-plugin/.claude-plugin/plugin.json`
- `model-check-plugin/steps/g2-logika.md`, `model-check-plugin/steps/g2-confirmation.md`

Коммиты: `08c3229` (feature до кода), `0bd2249` (сервер, guard, бюджеты, manifest, serve, wiring), `c0321ee` (правки по ревью), плюс коммит этого подтверждения и ревью.
