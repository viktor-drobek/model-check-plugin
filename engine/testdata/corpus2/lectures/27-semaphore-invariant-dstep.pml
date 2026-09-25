/*
 * Source: books-md/lect01-lect09.md, lines 4554-4560 (lecture slide: the cheapest invariant, a guarded d_step)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4554, 4560.
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
    d_step { !(count <= 1) -> assert(count <= 1)}
}
