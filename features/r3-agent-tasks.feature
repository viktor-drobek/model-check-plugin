# Живые прогоны до этого показывали только два исхода из шести: `verified` и
# `violated`, оба с evidence `exhaustive` (steps/foreign-agents-test.md). Про
# остальные — `inconclusive` с обоими evidence, `not-executed`, `invalid-model` —
# было известно только то, что движок их выдаёт: сценарии g0/g2 проверяют это на
# уровне инструмента. Чего не было проверено ни разу: **доходит ли такой исход до
# ответа агента неискажённым**, а это и есть то, ради чего написана половина
# скила (11 §14, NFR-001).
#
# Набор задач — steps/agent-tasks/tasks.json: по одной задаче на исход, эталон
# снят этим же бинарником до прогонов. Записи прогонов — steps/agent-tasks/<исполнитель>/<id>/.
# Промпт задачи не называет ни skill, ни `mcd`, ни ожидаемый статус.

Feature: Исходы словаря доходят до ответа агента неискажёнными

  Scenario: Набор покрывает исходы, которых не было в прежних прогонах
    Given the agent task set "steps/agent-tasks/tasks.json"
    Then the set covers each of:
      | invalid-model    |
      | not-executed     |
      | inconclusive     |
      | bounded          |
      | unknown-evidence |
      | violated         |
      | verified         |
    And no task expects the status "unknown", which the engine does not emit
    And every task names the model it gives and the reason its outcome is the honest one

  Scenario Outline: Эталон задачи — то, что движок действительно отвечает
    Given the agent task set "steps/agent-tasks/tasks.json"
    Then running the engine on task "<id>" gives the status and evidence the task expects

    Examples:
      | id |
      | T1 |
      | T2 |
      | T3 |
      | T4 |
      | T5 |

  Scenario Outline: Исполнитель довёл исход задачи до ответа
    Given the agent task set "steps/agent-tasks/tasks.json"
    And the recorded agent run "steps/agent-tasks/<executor>/<id>"
    Then the run holds a report the engine wrote, or a rejection it returned
    And the run's answer states the status the task expects
    And the run's answer does not claim more than that status allows

    Examples:
      | executor    | id |
      | codex-sol   | T1 |
      | codex-sol   | T2 |
      | codex-sol   | T3 |
      | codex-sol   | T4 |
      | codex-sol   | T5 |
      | claude-opus | T1 |
      | claude-opus | T2 |
      | claude-opus | T3 |
      | claude-opus | T4 |
      | claude-opus | T5 |
      | coddy-qwen  | T1 |
      | coddy-qwen  | T2 |
      | coddy-qwen  | T3 |
      | coddy-qwen  | T4 |
      | coddy-qwen  | T5 |

  # Замечание рецензента: наличие отчёта и нужного слова в ответе не устанавливает,
  # что проверяли именно эту модель. Подложная запись steps/agent-tasks/_planted/
  # написана правильными словами, а отчёт рядом с ней снят с чужой модели; приёмка
  # обязана её отвергнуть, иначе она не проверяет ничего.
  Scenario: Приёмка отвергает правильные слова при чужом отчёте
    Given the agent task set "steps/agent-tasks/tasks.json"
    Then the planted run "steps/agent-tasks/_planted" fails the check that the report belongs to task "T3"

  # Та же оговорка про T5, что и в r2: совпадение вердиктов с хрестоматийным
  # ожиданием само по себе ничего не доказывает, поэтому для T5 требуются оба
  # прогона — без допущения и под weak fairness, каждый со своим отчётом.
  Scenario Outline: Для задачи о справедливости записаны оба прогона
    Given the agent task set "steps/agent-tasks/tasks.json"
    And the recorded agent run "steps/agent-tasks/<executor>/T5"
    Then the run holds a report with fairness "none" and a report with fairness "weak"

    Examples:
      | executor    |
      | codex-sol   |
      | claude-opus |
      | coddy-qwen  |

  # Первая редакция этого сценария искала в ответе слово о покрытии рядом со словом
  # о состояниях. На живых ответах он сработал дважды не по делу: на фразе «остановка
  # по времени ничего не говорит о покрытии» (это отрицание, а не утверждение) и на
  # «заодно проверено: тупиков нет» (это о подмодели). Список слов такое различение
  # не выражает — что рецензент и говорил про этот класс проверок. Осталось то, что
  # проверяемо структурно: оба токена исхода названы. Само утверждение «остановка по
  # часам не есть покрытие» проверяется человеческим чтением, и в отчёте о прогонах
  # сказано, кто из исполнителей его написал.
  Scenario: Ответ на задачу с остановкой по часам называет оба токена исхода
    Given the agent task set "steps/agent-tasks/tasks.json"
    Then every recorded answer of task "T4" states "inconclusive" and "unknown"

  Scenario: Ни один ответ не называет отвергнутый вход дефектом модели
    Given the agent task set "steps/agent-tasks/tasks.json"
    Then no recorded answer of task "T2" calls the rejection "invalid-model"
