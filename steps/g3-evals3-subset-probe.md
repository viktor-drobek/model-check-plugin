# G3 — построчная сверка подмножества Promela с движком (evidence)

Поручение координатора: закрыть открытый пункт §6 `steps/g3-evals3-addendum.md` —
проверить **каждую** строку `skills/model-check/references/promela-subset.md` §3 и §1
пробой живого движка, в обе стороны: не только «то, что обещано вне подмножества,
действительно отвергается», но и «то, что обещано внутри, действительно принимается».
Таблица ниже — не пересказ подтверждений других шагов, а вывод команд.

## Как воспроизвести

```
cd model-check-plugin/engine && go build -o /tmp/mcd ./cmd/mcd
cd ../evals-workspace/subset-probes && python3 probe.py --mcd /tmp/mcd
```

`probe.py` для каждой строки справки пишет минимальную модель, которая упражняет
**ровно одну** конструкцию (носитель — простой Promela из MVP, чтобы отказ мог прийти
только от проверяемой конструкции), запускает `mcd parse --promela` и записывает
исход. Модели остаются на диске рядом со скриптом; `results.json` содержит код
возврата, `kind`, сообщение и предупреждения каждой пробы. Скрипт **не** считает
справку истинной: он печатает `ok` / `DISAGREE`, сравнивая заявленное с наблюдаемым,
поэтому разошедшаяся справка видна как расхождение, а не как пройденный тест.

Движок: `mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)`, собранный из дерева на
G5-дополнении + G6-упаковке. **58 проб.** При первом прогоне — 11 расхождений со
справкой; они разобраны ниже и справка исправлена, поэтому колонка «Reference says»
в таблице — это уже **исправленная** классификация, и чистый прогон даёт
**0 расхождений**. Любое расхождение в будущем означает, что движок и справка снова
разошлись: тогда надо перевыводить строку справки, а не править `claim` в скрипте,
чтобы он согласился.

## Три класса исхода

`mcd parse` различает больше, чем «внутри / вне», и сверка это показала:

1. **parsed** — конструкция внутри подмножества, что бы справка ни говорила.
2. **rejected** — отвергнута; важно, каким `kind`: §3 обещал `outside-subset`, но
   `P[i]:var` приходит как `syntax` (см. расхождения).
3. **parsed + warning** — принята, но семантика не та, которую даёт SPIN. Ровно один
   такой случай, и он содержательный: `pc_value` принимается и предупреждает, что
   нумерация управляющих точек у движка своя, не pan-овская.

Плюс отдельное значение `claim` = **`defect`**: движок конструкцию разбирает, но это
не граница подмножества, а известный дефект (§C). Скрипт печатает такие строки
отдельной строкой итога, чтобы «принято» не читалось как «поддержано».

| Probe | Construct | Reference says | `mcd parse` | Engine message (truncated) |
|---|---|---|---|---|
| `in-proctype-active-init.pml` | proctype, active proctype, active [N] proctype, init | inside | parsed |  |
| `in-params-pid.pml` | proctype parameters, _pid | inside | parsed |  |
| `in-run-in-loop.pml` | run inside a loop (G5) | inside | parsed |  |
| `in-run-recursive.pml` | recursive run (G5) | inside | parsed |  |
| `in-types-scalar.pml` | bit, bool, byte, short, int, pid | inside | parsed |  |
| `in-types-array.pml` | one-dimensional array of fixed length | inside | parsed |  |
| `in-mtype-eq-form.pml` | mtype = { a, b } | inside | parsed |  |
| `in-mtype-brace-form.pml` | mtype { a, b } | inside | parsed |  |
| `in-chan-buffered.pml` | chan c = [N] of { t }; send and receive | inside | parsed |  |
| `in-chan-rendezvous.pml` | chan c = [0] of { t } (rendezvous) | inside | parsed |  |
| `in-chan-multifield.pml` | c!e1,e2 and c!e1(e2) forms | inside | parsed |  |
| `in-chan-recv-constant.pml` | c?x with a constant (pattern match) | inside | parsed |  |
| `in-chan-predicates.pml` | len, empty, full, nempty, nfull | inside | parsed |  |
| `in-chan-xr-xs.pml` | xr c / xs c as hints | inside | parsed |  |
| `in-control.pml` | if/fi, do/od, ::, ->, else, break, goto, labels | inside | parsed |  |
| `in-label-prefixes.pml` | end, progress, accept label prefixes | inside | parsed |  |
| `in-atomic-dstep.pml` | atomic { … }, d_step { … } | inside | parsed |  |
| `in-statements.pml` | assignment, guard, assert, skip, true, false, printf | inside | parsed + warning | warning: printf: 1 statement(s) kept as no-op steps so that state counts match SPIN; their output is not produced |
| `in-timeout.pml` | timeout | inside | parsed |  |
| `in-define-object.pml` | #define NAME body | inside | parsed |  |
| `in-define-function.pml` | #define NAME(args) body | inside | parsed |  |
| `in-define-continuation.pml` | #define with a \ continuation | inside | parsed |  |
| `in-ifdef-family.pml` | #ifdef/#ifndef/#if/#elif/#else/#endif, defined(), #undef | inside | parsed |  |
| `in-never-claim.pml` | never { … } | inside | parsed |  |
| `in-expressions.pml` | arithmetic, comparison, &&, ||, !, % | inside | parsed |  |
| `in-remote-label-in-never.pml` | remote label reference P@label inside a never claim | outside | rejected `outside-subset` | construct outside subset: remote reference (P@label) (outside the subset) (in-remote-label-in-never.pml, line 8) |
| `out-inline.pml` | inline name(args) { … } | inside | parsed |  |
| `out-typedef.pml` | typedef | inside | parsed |  |
| `out-provided.pml` | provided (e) | inside | parsed |  |
| `out-chan-in-message.pml` | channels as message fields | inside | parsed |  |
| `out-chan-array.pml` | arrays of channels | inside | parsed |  |
| `out-chan-uninitialised.pml` | uninitialised channel variables | inside | parsed |  |
| `out-nr-pr.pml` | _nr_pr | inside | parsed |  |
| `out-unless.pml` | unless | outside | rejected `outside-subset` | construct outside subset: unless (plan 14 §5.2: outside the subset) (out-unless.pml, line 5) |
| `out-c-code.pml` | c_code | outside | rejected `outside-subset` | construct outside subset: c_code (embedded C is outside the subset) (out-c-code.pml, line 4) |
| `out-c-expr.pml` | c_expr | outside | rejected `outside-subset` | construct outside subset: c_expr (embedded C is outside the subset) (out-c-expr.pml, line 4) |
| `out-c-decl.pml` | c_decl | outside | rejected `outside-subset` | construct outside subset: c_decl (embedded C is outside the subset) (out-c-decl.pml, line 4) |
| `out-c-state.pml` | c_state | outside | rejected `outside-subset` | construct outside subset: c_state (embedded C is outside the subset) (out-c-state.pml, line 4) |
| `out-c-track.pml` | c_track | outside | rejected `outside-subset` | construct outside subset: c_code (embedded C is outside the subset) (out-c-track.pml, line 4) |
| `out-eval-in-receive.pml` | eval(e) in a receive | outside | rejected `outside-subset` | construct outside subset: eval (plan 14 §5.2: not in the corpus, outside the subset) (out-eval-in-receive.pml, line 5) |
| `out-priority.pml` | priority | outside | rejected `outside-subset` | construct outside subset: priority (process priorities are outside the subset) (out-priority.pml, line 4) |
| `out-bit-operators.pml` | bit operators | outside | rejected `outside-subset` | construct outside subset: bitwise operator & (outside the subset) (out-bit-operators.pml, line 5) |
| `out-shift-operators.pml` | shift operators | outside | rejected `outside-subset` | construct outside subset: bitwise operator << (outside the subset) (out-shift-operators.pml, line 5) |
| `out-conditional-expr.pml` | ?: conditional expression | outside | rejected `outside-subset` | construct outside subset: conditional expression (c -> a : b) (outside the subset) (out-conditional-expr.pml, line 5) |
| `out-run-in-expression.pml` | run as the whole right-hand side (pid = run P()) | inside | parsed |  |
| `out-run-nested-in-expression.pml` | run nested inside a larger expression | outside | rejected `outside-subset` | construct outside subset: run inside an expression (run is accepted as a statement, on its own or as `pid = run P(...)`, not inside a larger expressio |
| `out-remote-variable.pml` | remote variable reference P[i]:var | outside | rejected `syntax` | expected ";" or "->" after a statement, got : (out-remote-variable.pml, line 5) |
| `out-poll-receive.pml` | c?[…] poll | outside | rejected `outside-subset` | construct outside subset: channel poll (?[…]) (outside the subset) (out-poll-receive.pml, line 5) |
| `out-sorted-receive.pml` | c?? random/sorted receive | outside | rejected `outside-subset` | construct outside subset: random receive (??) (outside the subset) (out-sorted-receive.pml, line 5) |
| `out-copy-receive.pml` | c?<…> copy receive | outside | rejected `outside-subset` | construct outside subset: copy receive (?<…>) (outside the subset) (out-copy-receive.pml, line 5) |
| `out-unsigned.pml` | unsigned | outside | rejected `outside-subset` | construct outside subset: unsigned (outside the subset) (out-unsigned.pml, line 4) |
| `out-hidden-qualifier.pml` | hidden qualifier | outside | rejected `outside-subset` | construct outside subset: hidden (variable qualifiers are outside the subset) (out-hidden-qualifier.pml, line 4) |
| `out-local-qualifier.pml` | local qualifier | outside | rejected `outside-subset` | construct outside subset: local (variable qualifiers are outside the subset) (out-local-qualifier.pml, line 4) |
| `out-show-qualifier.pml` | show qualifier | outside | rejected `outside-subset` | construct outside subset: show (variable qualifiers are outside the subset) (out-show-qualifier.pml, line 4) |
| `out-ltl-block.pml` | ltl name { … } block | outside | rejected `outside-subset` | construct outside subset: ltl (inline LTL blocks are outside the subset; use never { }) (out-ltl-block.pml, line 6) |
| `out-pc-value.pml` | pc_value(pid) | inside | parsed + warning | warning: pc_value (line 6): the value is this engine's control-location numbering, which is built differently from pan's internal state numbers; a mod |
| `out-include.pml` | #include | outside | rejected `outside-subset` | construct outside subset: #include (the model must be a single file) (out-include.pml, line 4) |
| `out-block-redeclaration.pml` | block-scoped redeclaration of a local | defect | parsed |  |

## Разбор одиннадцати расхождений

### A. Справка обещала больше, чем движок делает (1) — правка справки

**`P@label` внутри never claim.** §1 «Expressions» обещала удалённые ссылки на метки.
Движок отвергает: `outside-subset`, «remote reference (P@label)». Это **не дефект
движка**: план 14 §5.2 удалённых ссылок нигде не обещает — ни в MVP, ни в v1, — так
что переобещала справка, а движок отвергает ровно то, что план и не включал.
Строка перенесена из §1 в §3.

Отдельно и важно для отчётов: `P@label` **принимается в CTL** (`--ctl 'AG EF
(subscriber@Idle)'` на `CH14/version1` → `verified`, атом нормализуется в
`pc(0) == 0`), отвергается в `--ltl` и в never claim. Это уже описано в
`properties-ltl-ctl.md` §2 тремя маршрутами.

### B. Движок расширился, справка отстала (9) — правка справки

G5 внёс в подмножество то, что план 14 §5.2 относил к v1. Приняты пробами:
`inline`, `typedef`, `provided (e)`, каналы в сообщениях, массивы каналов,
неинициализированные каналы, `_nr_pr`, `pc_value`, и `run` в правой части присваивания.

Две строки требуют не «переноса», а **уточнения**, иначе справка соврёт в другую сторону:

- **`run` внутри выражения.** Принимается `pid = run P(...)` (движок кладёт в
  переменную новый pid) — проба `out-run-in-expression.pml`. Отвергается `run` внутри
  **большего** выражения — проба `out-run-nested-in-expression.pml` (`n = 1 + run Q()`);
  сообщение движка прямо это и разделяет: «run is accepted as a statement, on its own
  or as `pid = run P(...)`, not inside a larger expression». Корпусный
  `CH3/notpossible.pml` (`!run A()`, `_pid > 0 && - run A()`) по-прежнему отвергается,
  и SPIN его тоже не принимает. То есть прежняя строка была не устаревшей, а слишком
  широкой, и обе половины теперь закреплены отдельными пробами.
- **`pc_value`.** Принимается, но с предупреждением: нумерация управляющих точек у
  движка своя. `CH4/pcval.pml` (G4 числил его вне подмножества) разбирается, выход 0,
  три предупреждения. Для отчёта это значит: модель, поведение которой зависит от
  конкретного числа `pc_value`, ведёт себя здесь иначе, чем под SPIN, — принята, но
  переносить вердикт на SPIN нельзя. Строка идёт в §2 («расхождения с SPIN»), а не в §3.

### C. Дефект движка (1) — **не заделывается документацией, отчёт координатору**

**Переобъявление локальной переменной в блоке принимается молча, и две переменные
сливаются в одну.**

```
$ cat out-block-redeclaration.pml
active proctype P() { byte n; n = 1; { byte n; n = 2 } }

$ /tmp/mcd parse --promela out-block-redeclaration.pml        # exit 0
   процесс P:0, locals: ['n']          ← одна переменная вместо двух

$ spin -a out-block-redeclaration.pml
spin: out-block-redeclaration.pml:4, Error: redeclaration of 'n'
```

SPIN 6 файл **отвергает**. Движок его принимает и строит модель, в которой внутренняя
`n` — это внешняя `n`: присваивание в блоке затирает внешнюю переменную. Справка
обещала «outside, with a message»; сообщения нет, отказа нет, есть молчаливая подмена
модели.

Дефект именно в **переобъявлении**, а не в блочной области видимости вообще: на
`CH3/scope.pml` (переменная объявлена во вложенном блоке и используется после него)
движок даёт `semantic` «undeclared variable y (line 11)», и **SPIN даёт ту же ошибку
на той же строке** — тут движок прав. Расходятся они ровно там, где имя переобъявлено. Это тот же класс, что найденный ранее `goto` на несуществующую метку: модель,
которую проверяют, — не та, которую написал пользователь, и ни кода возврата, ни
`warnings` об этом не говорят. Ожидаемое поведение — отказ `kind: semantic` с именем
переменной и строкой. Владелец `engine/frontend/promela` — агент G5.

### D. Уточнение, не расхождение: род отказа

`P[0]:v` (удалённая ссылка на переменную) отвергается с `kind: "syntax"`
(«expected ";" or "->" after a statement, got :»), а не `outside-subset`. Преамбула §3
обещала, что «парсер отвергает это с `kind: outside-subset`», и учила различать:
`syntax`/`semantic` — ошибки модели, которые чинят, а `outside-subset` — граница,
которую объясняют. Для `P[i]:var` читатель по этому правилу сделает неверный вывод
(«я опечатался»), тогда как конструкция просто не поддержана. В §3 это теперь сказано
в самой строке.

## Что осталось верным без правок

46 из 58 проб подтвердили справку как есть: все процессы, типы, массивы, обе формы
`mtype`, каналы и их предикаты, `xr`/`xs` как хинты, управляющие конструкции и
префиксы меток (`accept` и `progress` дают свойства `accept`/`progress` в IR),
`atomic`/`d_step`, операторы, `timeout`, весь препроцессор с `-D`, арифметика и
логика, и все конструкции, которые план исключает совсем: `unless`, вся семья
`c_*`, `eval`, `priority`, битовые и сдвиговые операторы, `?:`, приём с опросом /
сортировкой / копированием, `unsigned`, квалификаторы `hidden`/`show`/`local`,
блоки `ltl { }`, `#include`.
