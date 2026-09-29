# Открытый с G2 вопрос: «вызов mc_* самим скиллом в установленном плагине» —
# steps/g6-confirmation.md §7.3 фиксирует его как невыполненный, потому что
# субагент не может зарегистрировать MCP-сервер в свою сессию. Сценарии ниже
# принимают не «сервер запускается» (это делают g2-mcp и g6-package), а именно
# **сквозной маршрут**: агент, которому дан только skill, дошёл до движка через
# MCP-инструменты, и отчёт, которым он ответил, написан сервером на модели
# задания.
#
# Записей две, и они отвечают на разные половины вопроса:
#   coddy-qwen38-mcp — внешний клиент (Coddy, модель qwen3.8-27b), объявивший
#     сервер тем же файлом, что и поставка; показывает, что маршрут не зависит
#     от клиента, для которого плагин писался;
#   claude-code-mcp — тот самый клиент (Claude Code, `--plugin-dir`), где
#     регистрация идёт штатным путём из plugin.json.
# Проверяются записи, а не живой запуск: клиенты внешние и в CI их нет, живые
# прогоны описаны в steps/foreign-agents-test.md.

Feature: Сквозной маршрут skill → MCP на реальных клиентах

  Scenario: Внешний клиент объявил сервер поставочным файлом
    Given the recorded MCP run "steps/foreign-agents-test/coddy-qwen38-mcp"
    Then the run's "mcp.json" declares the server "model-check"
    And that declaration's command and args match "mcp/servers.json" with CLAUDE_PLUGIN_ROOT expanded

  Scenario: В Claude Code сервер пришёл из манифеста плагина, а не из отдельного конфига
    Given the recorded MCP run "steps/foreign-agents-test/claude-code-mcp"
    Then ".claude-plugin/plugin.json" points its mcpServers at "./mcp/servers.json"
    And the run's tool log names each of:
      | mcp__plugin_model-check_model-check__mc_parse |
      | mcp__plugin_model-check_model-check__mc_check |

  Scenario Outline: Агент дошёл до движка через MCP-инструменты, а не через CLI
    Given the recorded MCP run "<run>"
    Then the run's tool log names each of:
      | mc_parse |
      | mc_check |
    And the run's tool log records no shell invocation of the "mcd" binary

    Examples:
      | run                                             |
      | steps/foreign-agents-test/coddy-qwen38-mcp      |
      | steps/foreign-agents-test/claude-code-mcp       |

  Scenario Outline: Отчёт, которым отвечает агент, написан сервером на модели задания
    Given the recorded MCP run "<run>"
    Then the run holds a report the engine wrote
    And that report's input hash is the hash of the run's "model.pml"
    And that report has the property "assert" with status "violated" and evidence "exhaustive"

    Examples:
      | run                                             |
      | steps/foreign-agents-test/coddy-qwen38-mcp      |
      | steps/foreign-agents-test/claude-code-mcp       |

  Scenario Outline: Сессия сервера принадлежит этому прогону
    Given the recorded MCP run "<run>"
    Then the run holds the server's session manifest
    And that manifest records a call of "mc_check"

    Examples:
      | run                                             |
      | steps/foreign-agents-test/coddy-qwen38-mcp      |
      | steps/foreign-agents-test/claude-code-mcp       |

  # Вердикт засчитывается и по-русски: сценарий проверяет происхождение ответа, а
  # не орфографию. Что запись Coddy не сохранила английский токен `violated`
  # (хотя `exhaustive` сохранила) — отступление от SKILL.md шага 8; оно записано
  # наблюдением в steps/foreign-agents-test.md §5, а не спрятано сюда.
  Scenario Outline: Ответ агента излагает вердикт этого отчёта
    Given the recorded MCP run "<run>"
    Then the run's "answer.md" states the status of the property "assert" from that report
    And the run's "answer.md" names the interleaving in which both clients pass the flag test

    Examples:
      | run                                             |
      | steps/foreign-agents-test/coddy-qwen38-mcp      |
      | steps/foreign-agents-test/claude-code-mcp       |

  # То, чего вторая запись достигает, а первая нет. Оба пункта — требования самого
  # skill, и здесь они предъявлены выполненными на живом прогоне, а не в тексте.
  Scenario: Свой список свойств не вытеснил свойства модели
    Given the recorded MCP run "steps/foreign-agents-test/claude-code-mcp"
    Then the run holds a report the engine wrote
    And that report has the property "deadlock" with status "verified" and evidence "exhaustive"
    And that report has the property "assert" with status "violated" and evidence "exhaustive"

  Scenario: Словарь статусов сохранён по-английски
    Given the recorded MCP run "steps/foreign-agents-test/claude-code-mcp"
    Then the run's "answer.md" keeps the tokens "violated" and "exhaustive"
