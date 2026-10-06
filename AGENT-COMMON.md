# Общие инструкции для агентов сборки (model-check skill)

Repo root: the repository checkout selected by the operator.
Branch: the work branch selected by the operator; do not push, switch branches, or touch files owned by other agents.
Plugin dir: model-check-plugin/ (engine/ = Go-модуль `modelcheck`, Go 1.26; skills/model-check/; features/; steps/).

Прочитать полностью до начала работы:
1. model-check-plugin/BUILD-PROTOCOL.md — обязательный протокол шага из шести пунктов (Cucumber сначала → реализация → юнит-тесты → зелёный godog → логическое ревью → подтверждение).
2. model-check-skill-notes/14-skill-building-plan.md — план (§3 архитектура, §4 объём движка по версиям, §5 входные форматы, §6 MCP-инструменты, §7 skill, §8 тестирование, §9 порядок сборки и критерии выхода, §12 допущения).
3. model-check-skill-notes/11-skill-requirements.md §14 (словарь статусов) и цитируемые FR/NFR.
4. Skill logika для пункта 5 протокола: прочитать установленный `logika` skill (`SKILL.md` и `references/errors.md`) из каталога skill host; не фиксировать абсолютный путь конкретной машины.
5. Подтверждения уже закрытых шагов в model-check-plugin/steps/*-confirmation.md — они фиксируют принятые решения (K1, коды выхода, правила статусов).

Godog-харнесс: engine/features_test.go прогоняет все ../features/*.feature (тег ~@pending, strict). Step-определения — в engine/steps_<step>_test.go, добавляющих регистратор в `stepRegistrars` в init(); features_test.go не менять. Окружение для go: `export GOFLAGS=-mod=mod GOPROXY=https://proxy.golang.org,direct GOTOOLCHAIN=local`. Новые зависимости — только названные планом (MCP SDK) или test-only; каждую фиксировать в подтверждении.

Корпус моделей: "Promela - examples/<CHn>/<file>" — только чтение; ссылаться по пути. SPIN 6.5.2 установлен: /usr/bin/spin.

Детерминизм: одинаковый вход → одинаковый JSON/текст. Никаких map-итераций в выводе.

Коммиты: `git add` только своих файлов; сообщение начинается с идентификатора шага (например "G1: ..."), заканчивается строкой
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Коммитить минимум в конце шага, чаще — можно. Никогда не `git push`, не amend, не rebase.

В конце вернуть: вердикт по критерию выхода (выполнен / частично / не выполнен), сводку тестов (go test + godog), замеры, созданные файлы, что отложено и почему, рекомендации, требующие правки плана 14 (план самому НЕ править).
