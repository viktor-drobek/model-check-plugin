/*
 * Source: books-md/lect01-lect09.md, lines 2543-2543 (lecture slide: mutual exclusion on a single busy flag)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bool busy;
byte mutex;

proctype P(bit i) {
    (!busy) ->
    busy = true;
    mutex++;
    printf(P%d in critical section\n',i);
    mutex--;
    busy = false
}

active proctype invariant() {
    assert(mutex <= 1)
}

init {
    atomic {
        run P(0);
        run P(1)}
}
