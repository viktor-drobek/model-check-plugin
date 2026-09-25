/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 3905-3905 (§5.10, a data race on a shared byte through a local temporary)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte state = 1;

proctype A() {
    byte tmp;
    (state==1) ->
    tmp = state;
    tmp = tmp+1;
    state = tmp
}

proctype B() {
    byte tmp;
    (state==1) ->
    tmp = state;
    tmp = tmp-1;
    state = tmp
}

init {
    run A();
    run B()
}
