/*
 * Source: books-md/lect01-lect09.md, lines 2619-2619 (lecture slide: Peterson's mutual-exclusion algorithm (1981))
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «Сигнал о входе/выходе из критичесой секции Число процессов в критической секции Чей ход?»
 */
mtype = {A_Turn, B_Turn};
bool x,y;
byte mutex;
mtype turn = A_Turn;

active proctype A() {
    x = true;
    turn = B_Turn;
    (!y || turn == A_Turn) ->
    mutex++;
    /*critical section*/
    mutex--;
    x = false;
}

active proctype invariant() {
    assert(mutex <= 1)
}

active proctype B() {
    y = true;
    turn = A_Turn;
    (!x || turn == B_Turn) ->
    mutex++;
    /*critical section*/
    mutex--;
    y = false;
}
