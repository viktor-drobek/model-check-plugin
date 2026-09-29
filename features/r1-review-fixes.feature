# Правки по внешнему ревью: steps/review-astra-plan.md, steps/review-astra-skill.md
# и согласование steps/review-sol-reconciliation.md. Ревью нашло не ошибки движка,
# а расхождение инструкций скила с движком после G5/G6: skill одновременно запрещал
# работающие возможности (CTL, `provided`) и обещал отсутствующие (дерево CTL,
# replay по id контрпримера, покрытие «всё до глубины D»). Сценарии ниже
# закрепляют исправленные формулировки дословно — чтобы неверная фраза не
# вернулась копипастом, а верная не исчезла молча.

Feature: Инструкции скила согласованы с движком после ревью

  Scenario: `bounded` больше не обещает покрытия, которого поиск не даёт
    Then "skills/model-check/references/evidence-and-status.md" contains:
      | What `bounded` does not mean                                      |
      | it does not say that everything below the bound was searched      |
      | stored but not expanded                                           |
      | a property can be decided — `violated` with a real counterexample — at depth D+1 |
      | never "everything up to <bound> was checked"                      |
    And "skills/model-check/references/evidence-and-status.md" no longer contains:
      | the search covered everything up to an explicit bound and nothing beyond |
      | "everything up to depth D" are statements a reader can act on           |
      | experimental LTL (plan §11)                                             |
      | kind `ctl` not executed by this build                                   |

  Scenario: Регрессионная проверка `reach` читается в терминах движка
    Then "skills/model-check/references/counterexamples.md" contains:
      | the `reach bad` property becomes **`violated`**       |
      | that `violated` is the answer you wanted             |
      | so the word carries no evaluation of its own         |
      | a different property, not the same one read differently |
    And "skills/model-check/references/counterexamples.md" no longer contains:
      | the `reach` property should become `verified` on `[] !bad` (unreachable) |

  Scenario: Декодирование трассы отделено от её воспроизведения
    Then "skills/model-check/references/counterexamples.md" contains:
      | `mc_explain` **decodes** the trace the check already stored |
      | There is no call that replays a counterexample by its id    |
      | `mode: "guided"`                                            |
    And "skills/model-check/references/workflow.md" contains:
      | `violated` only after `mc_explain` decoded the trace |
    And "skills/model-check/references/workflow.md" no longer contains:
      | `violated` only after `mc_explain` replayed the trace |
    And "skills/model-check/references/counterexamples.md" no longer contains:
      | the trace replayed by `mc_simulate` in `guided` mode from the counterexample id |

  Scenario: Вердикты без прогона названы, и explain не требуется для них
    Then "skills/model-check/SKILL.md" contains:
      | that carries a `counterexample`                          |
      | Some verdicts carry no run at all                        |
      | `temporal.witness_note`                                  |
    And "skills/model-check/references/counterexamples.md" contains:
      | the engine builds no tree             |
      | **no run at all**                     |
    And "skills/model-check/references/counterexamples.md" no longer contains:
      | and, per branch, why it fails |

  Scenario: Ни один файл скила не запрещает CTL, который движок исполняет с G5
    Then "skills/model-check/SKILL.md" no longer contains:
      | It does not check CTL yet |
    And "skills/model-check/references/properties-ltl-ctl.md" no longer contains:
      | this build does not check CTL at all |
    And "skills/model-check/references/properties-ltl-ctl.md" contains:
      | a `ctl` property asked with `fairness: weak` or `strong` comes back `not-executed` |
    And "skills/model-check/SKILL.md" contains:
      | `AG EF switch@Idle` comes back `verified` / `exhaustive` |
      | `provided` entered the subset in v1 (G5)                 |

  Scenario: Примеры синтаксиса CTL — те, что парсер принимает
    Then "skills/model-check/references/properties-ltl-ctl.md" contains:
      | `proc:pid@label`                          |
      | The bracket form `proc[i]@label` is *not* parsed |
      | `E[p U q]`, `A[p U q]`                    |
    And "skills/model-check/references/properties-ltl-ctl.md" no longer contains:
      | `E(p U q)`, `AG EF p` — **is executed since G5** |

  Scenario: Живость перехода сети Петри записана выражением, которое исполняется
    Then "skills/model-check/references/petri-nets.md" contains:
      | There is no `fire(t)` atom                       |
      | `AG EF (p1 >= 1)` runs and comes back `violated` |
      | For the LTL row it is **not** exact              |
      | is *offered* infinitely often                    |
    And "skills/model-check/references/petri-nets.md" no longer contains:
      | today neither can be run |

  Scenario: Отвергнутый вход и переполнение домена разведены по статусам
    Then "skills/model-check/SKILL.md" contains:
      | input rejected by the frontend         |
      | Not the same as a rejected input       |
    And "skills/model-check/references/promela-subset.md" contains:
      | The **status** is still `not-executed` |
      | *not* a send into a full channel      |
    And "skills/model-check/references/promela-subset.md" no longer contains:
      | so the status is not `not-executed` |

  Scenario: Список свойств, отправленный в mc_check, заменяет свойства модели
    Then "skills/model-check/SKILL.md" contains:
      | **A property list you pass replaces the model's own** |
    And "skills/model-check/references/properties-ltl-ctl.md" contains:
      | Pass your own `properties` and it is gone with the rest of them |
    And "skills/model-check/references/properties-ltl-ctl.md" no longer contains:
      | it appears in every report for that model whether or not you asked for it |

  Scenario: Границы поставки и воспроизводимости названы честно
    Then "skills/model-check/references/engine-tools.md" contains:
      | validated with a real client on **linux/arm64** |
      | for runs that were not stopped by the clock     |
      | `mcd check --estimate`                          |
    And "skills/model-check/references/engine-tools.md" no longer contains:
      | is a crude stand-in |
    And "skills/model-check/references/workflow.md" contains:
      | `mc_manifest` takes **only** `session_id` |
    And "skills/model-check/references/workflow.md" no longer contains:
      | All are inputs to `mc_manifest` |

  Scenario: Ручной разбор strong fairness не подменяет статус движка
    Then "skills/model-check/references/fairness.md" contains:
      | **The status does not change**          |
      | is **not** an encoding of strong fairness |
    And "skills/model-check/references/fairness.md" no longer contains:
      | `violated` for the strong-fairness reading |

  Scenario: Факт вызова движка в evals проверяется его отчётом, а не текстом ответа
    Then "skills/model-check/evals/evals.json" contains:
      | "type": "engine_report" |
    And "skills/model-check/evals/evals.json" no longer contains:
      | "pattern": "mcd check\|mc_check" |
