/*
 * Source: books-md/lect01-lect09.md, lines 4576-4576 (lecture slide: valid end states marked with end: labels)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
mtype = {p, v};
chan sem = [0] of {mtype};
byte count;

active proctype semaphore() {
end:
    do
    :: sem!p -> sem?v
    od
}

active proctype user() {
end:
    do
    :: sem?p;
       count++;
       /*critical section*/
       count--;
       sem!v
    od
}
