# G5 — дополнение: stutter extension в произведении `np_`

Поручение координатора после мутационной кампании K3 (`steps/k3-mutation-report.md`,
`steps/k3-confirmation.md` §3.2; восемь сохранённых мутантов в
`engine/testdata/mutate/disagreements/CH4_dijkstra_progress/`).

Решение принято координатором из двух неравнозначных вариантов, которые назвал K3:
**отключить stutter extension внутри произведения с `np_`**, а не объявлять прежнее
поведение сознательным расхождением с SPIN.

## 1. Дефект

Каждая из восьми мутаций `CH4/dijkstra_progress.pml` блокирует систему: ни `Dijkstra`,
ни `user` не могут сделать ход. G4 ввёл stutter extension и применил его ко **всякому**
произведению: конечный прогон достраивается бесконечным повторением последнего состояния,
чтобы формулу можно было оценить на бесконечном следе. В произведении с `np_` это давало
бесконечный прогон, не проходящий ни одной `progress`-метки, то есть непрогрессивный цикл,
— и движок говорил `violated` там, где pan говорит `verified`: под `pan -l` у состояния без
разрешённых переходов преемников нет вовсе, цикла через него не бывает.

Расхождение — не только с pan. «Система застряла» и «система крутится, не продвигаясь» —
разные дефекты с разной починкой, и skill учит их различать (заметки 10 §10). Сообщать
первое как второе значит прятать тупик за ливнесс-вердиктом.

## 2. Что изменено

`engine/explore/cycle.go`: у `cycleSearch` появилось поле `noStutter`, оно ставится при
`prop.Kind == KindProgress` и больше нигде; обе ветки, порождавшие `productMove{stutter:
true}`, проверяют его. Шапка файла объясняет, почему два случая различаются: для LTL и
never claim достройка совпадает с pan и остаётся ровно там, где её поставил G4.

Тупик при этом **не исчезает из отчёта**: его сообщает безопасностный поиск, которому этот
вопрос и принадлежит. У всех восьми мутантов `deadlock` — `violated`, как и было.

## 3. Восемь мутантов: было и стало

`mcd check --promela <файл> --progress --sweep` против `pan -l -c0`
(`pandiff -mode l`, SPIN 6.5.2, `gcc -O2 -DNOREDUCE`):

| мутант | мутация | было: движок / pan | стало: движок / pan | состояния (движок = pan) |
|---|---|---|---|---|
| m001-invert-guard | строка 9, `(count == 1)` → `!(…)` | violated / verified | **verified / verified** | 1 |
| m002-invert-guard | строка 11, `(count == 0)` → `!(…)` | violated / verified | **verified / verified** | 26 |
| m004-off-by-one | строка 6, `1` → `2` | violated / verified | **verified / verified** | 1 |
| m005-off-by-one | строка 9, `1` → `2` | violated / verified | **verified / verified** | 1 |
| m007-off-by-one | строка 11, `0` → `1` | violated / verified | **verified / verified** | 26 |
| m008-off-by-one | строка 12, `1` → `2` | violated / verified | **verified / verified** | 40 |
| m009-drop-alternative | строка 9 | violated / verified | **verified / verified** | 1 |
| m010-drop-alternative | строка 11 | violated / verified | **verified / verified** | 24 |

Все восемь: `agree` по вердикту, классу ошибки и числу состояний. Свойство `deadlock` у
всех восьми — `violated` (тупик сообщён как тупик).

## 4. Побочная находка K3: два `run` с равными аргументами — **реальна, исправлена**

Файл `engine/testdata/mutate/state-count/you_run2-equal-run-arguments.pml`: мутант
`CH3/you_run2.pml`, где `init` запускает два экземпляра `you_run` с **одинаковым**
параметром. Вердикты совпадали, счётчики — нет: движок 14 состояний, pan 12.

Находка реальна, и причина — ровно тот инвариант, на котором держится вся кодировка
динамических процессов («k-й живой экземпляр proctype — это k-й слот его пула»).
G1 и первая редакция G5 давали `run`, про который фронтенд видит, что он берётся не более
одного раза, **собственный** экземпляр. Если первый экземпляр умирает до того, как сработал
второй `run`, pan заводит новый процесс на освободившемся pid, а выделенный второй слот
держит их врозь — и движок считает разными два состояния, которые pan считает одним.
Исправление: **всякий** `run` тянет из пула своего proctype, а движок берёт первый спящий
слот; размер пула не изменился, изменился только выбор в момент срабатывания.

- мутант: движок 12 = pan 12 (было 14);
- оригинал `CH3/you_run2.pml`: 14 = 14 (как было);
- `CH3/euclid.pml` 10, `CH3/typedef.pml` 5, `CH14/version4` 46 825, `CH9/leader.pml` 41 692,
  `CH15/client_server.pml` 191 200 — без изменений.

Инвариант теперь держится и в этом углу: живые экземпляры proctype всегда занимают префикс
пула, потому что умирает только самый молодой процесс, а он же — самый поздний занятый слот.

## 5. Тесты

```
gofmt — чисто; go vet ./... — чисто
go test ./... (кроме tools/pandiff) — ok
  новые: explore.TestBlockedStateHasNoSuccessorInTheNPProduct — тупиковое состояние даёт
         deadlock violated и progress verified, без контрпримера, и так же без метки
         progress (правило о преемниках, а не о метках);
         explore.TestStutterExtensionStaysForLTL — на той же заблокированной модели
         `[](x == 1)` по-прежнему violated с лассо;
  обновлены под пул: promela.TestRunAllocationOrderMatchesPan,
         promela.TestRunInstancesAndEndGuards (G1)
go test ./tools/pandiff -run 'TestDifferentialCorpus|TestDifferentialTriples' — ok, 148 с
  TestDifferentialCorpus: 51 модель, все agree (108 с)
  TestDifferentialTriples: 43 тройки G4, все agree (41 с). Четыре строки `np_` в них —
    CH4/fair.pml none/weak (violated 4 / violated 10) и CH4/dijkstra_progress.pml
    none/weak (verified 39 / verified 99) — не изменились: эти модели не блокируются,
    и правило о преемниках их не касается
godog (общий харнесс, -count=1, 1 мин 57 с): 311 сценариев: 301 passed, 10 undefined;
  — все 42 сценария features/g5-ctl-v1.feature PASS, @spin включительно, в том числе
    два новых: «a deadlocked system is a deadlock, not a non-progress cycle» (на m004)
    и «the stutter extension stays where G4 put it, for ltl»;
  — 10 undefined и 7 упавших сценариев принадлежат features/g6-package.feature —
    файлу агента G6, который появился в дереве во время этого дополнения и чьи шаги
    ещё не написаны. Ни одного отказа в моих файлах и в файлах закрытых шагов.
```

## 6. Что это меняет в документации (маршрутизация — за координатором)

Вердиктов, которые утверждает другой feature-файл, изменение не трогает: ни одна модель
корпуса в наборе не блокируется под `-l`, и обе строки `np_` в тройках G4 остались
прежними. Но два места в `skills/` (файлы агента G3, я их не правил) теперь говорят больше,
чем верно:

1. **`references/counterexamples.md`, §2a, пункт 1 «Stutter extension»** — там сказано, что
   шаг `-` со stutter в лассо значит «ничего дальше не происходит, и обещание не
   выполняется», и советуется «проверьте рядом свойство `deadlock` — stutter-петля на
   незавершённом состоянии есть тупик в костюме ливнесс-нарушения». Для `ltl` и never claim
   это по-прежнему верно. Для непрогрессивного поиска такой петли больше **не бывает**:
   тупик приходит как `deadlock`, а `progress` остаётся `verified`. Абзац стоит разделить
   по видам свойства.
2. **`references/properties-ltl-ctl.md`, §6, пункт 4 «Stutter extension»** — формулировка
   безусловная («прогон, который нельзя продолжить, достраивается»). Нужна оговорка: кроме
   поиска непрогрессивных циклов, где у состояния без разрешённых переходов преемников нет,
   как под `pan -l`. Ту же оговорку уместно повторить в §7 рядом со словами про `np_`.

`SKILL.md` (строка ~180, «the weak-fairness bookkeeping or the stutter extension of a system
that has stopped») остаётся верным: речь о чтении шагов `-`, а они по-прежнему бывают.

## 7. Файлы

- `model-check-plugin/engine/explore/{cycle,cycle_test}.go`
- `model-check-plugin/engine/frontend/promela/{lower,promela_test,promela_g5_test}.go`
- `model-check-plugin/features/g5-ctl-v1.feature`, `model-check-plugin/engine/steps_g5_test.go`
- `model-check-plugin/steps/g5-addendum-confirmation.md`

Доказательства K3 (`engine/testdata/mutate/…`) не трогались: они принадлежат K3 и служат
здесь входом тестов.
