/*
 * Source: books-md/lect01-lect09.md, lines 2569-2569 (lecture slide: mutual exclusion with two flags (mutex2.pml), deadlocks)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Same listing at markdown line 2569 (whole slide) and, split over two slides, at lines 2593 and 2599.
 * Dropped slide annotation: «Сигнал о входе/выходе из критичесой секции Число процессов в критической секции»
 */
bit x,y;
byte mutex;

active proctype A() {
    x = 1;
    (y == 0) ->
    mutex++;
    printf("%d\n", _pid);
    mutex--;
    x = 0;
}

active proctype invariant() {
    assert(mutex != 2)
}

active proctype B() {
    y = 1;
    (x == 0) ->
    mutex++;
    printf("%d\n",_pid);
    mutex--;
    y = 0
}
