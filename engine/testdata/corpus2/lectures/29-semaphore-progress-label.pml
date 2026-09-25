/*
 * Source: books-md/lect01-lect09.md, lines 4590-4590 (lecture slide: progress states marked with a progress: label)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
mtype = {p, v};
chan sem = [0] of {mtype};
byte count;

active proctype semaphore() {
    do
    :: sem!p -> progress: sem?v
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
