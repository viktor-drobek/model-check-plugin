# Открытый с G2 вопрос: «вызов mc_* самим скиллом в установленном плагине» —
# steps/g6-confirmation.md §7.3 фиксирует его как невыполненный, потому что
# субагент не может зарегистрировать MCP-сервер в свою сессию. Клиент, который
# это умеет, нашёлся снаружи: Coddy читает <workspace>/.coddy/mcp.json в том же
# формате, в каком плагин уже объявляет сервер. Сценарии ниже принимают не
# «сервер запускается» (это G2 и G6 проверяют сами), а именно **сквозной
# маршрут**: агент, которому дан только skill, дошёл до движка через MCP-
# инструменты, и отчёт, которым он ответил, написан сервером на модели задания.
#
# Предмет сценариев — запись прогона в steps/foreign-agents-test/coddy-qwen38-mcp/.
# Запись, а не живой запуск: клиент внешний и в CI его нет, поэтому приёмка
# проверяет артефакты, а живой прогон описан в steps/foreign-agents-test.md.

Feature: Сквозной маршрут skill → MCP на внешнем клиенте

  Background:
    Given the recorded MCP run "steps/foreign-agents-test/coddy-qwen38-mcp"

  Scenario: Сервер объявлен тем же файлом, что и в поставке плагина
    Then the run's "mcp.json" declares the server "model-check"
    And that declaration's command and args match "mcp/servers.json" with CLAUDE_PLUGIN_ROOT expanded

  Scenario: Агент дошёл до движка через MCP-инструменты, а не через CLI
    Then the run's tool log names each of:
      | mc_parse |
      | mc_check |
    And the run's tool log records no shell invocation of the "mcd" binary

  Scenario: Отчёт, которым отвечает агент, написан сервером на модели задания
    Then the run holds a report the engine wrote
    And that report's input hash is the hash of the run's "model.pml"
    And that report has the property "assert" with status "violated" and evidence "exhaustive"
    # `deadlock` в этом отчёте нет, и это не дефект записи: прогон передал
    # собственный список свойств, который заменяет свойства модели (SKILL.md шаг 6).
    # Требовать его здесь значило бы требовать другого прогона, а не другого движка.

  Scenario: Сессия сервера принадлежит этому прогону
    Then the run holds the server's session manifest
    And that manifest records a call of "mc_check"

  # Вердикт засчитывается и по-русски: проверяется происхождение ответа, а не
  # орфография. Сам факт, что запись не сохранила английский токен `violated`
  # (хотя `exhaustive` сохранила), — отступление от SKILL.md шага 8; оно записано
  # наблюдением в steps/foreign-agents-test.md §5, а не спрятано в этот сценарий.
  Scenario: Ответ агента излагает вердикт этого отчёта
    Then the run's "answer.md" states the status of the property "assert" from that report
    And the run's "answer.md" names the interleaving in which both clients pass the flag test
