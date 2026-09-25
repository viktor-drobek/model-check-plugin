/*
 * Source: books-md/lect01-lect09.md, lines 4510-4516 (lecture slide: system invariant as a one-shot assert process)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4510, 4516.
 */
mtype = {p, v};
chan sem = [0] of {mtype};
byte count;

active proctype semaphore() {
    do
    :: sem!p -> sem?v
    od
}

active proctype user() {
    do
    :: sem?p;
       count++;
       /*critical section*/
       count--;
       sem!v
    od
}

active proctype invariant() {
    assert(count <= 1)
}
