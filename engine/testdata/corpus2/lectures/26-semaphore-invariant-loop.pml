/*
 * Source: books-md/lect01-lect09.md, lines 4534-4540 (lecture slide: the same invariant as an endless do..od monitor)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4534, 4540.
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
    do
    ::assert(count <= 1)
    od
}
