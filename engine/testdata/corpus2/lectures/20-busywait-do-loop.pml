/*
 * Source: books-md/lect01-lect09.md, lines 4260-4260 (lecture slide: typical mistake, busy waiting in a do..od loop)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bit b;

active proctype {
    do
    :: b == 1 -> A
    :: else -> skip
    od
}
