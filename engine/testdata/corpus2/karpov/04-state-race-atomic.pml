/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 3911-3911 (§5.10, the same model with both bodies wrapped in atomic)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte state = 1;

proctype A() {
    atomic { (state==1) -> state = state+1 }
}

proctype B() {
    atomic { (state==1) -> state = state-1 }
}

init {
    run A();
    run B()
}
