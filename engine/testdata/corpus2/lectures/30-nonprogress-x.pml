/*
 * Source: books-md/lect01-lect09.md, lines 4602-4602 (lecture slide: two processes toggling one variable, non-progress cycles)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte x = 2;

active proctype A() {
    do
    :: x = 3 - x
    od
}

active proctype B() {
    do
    :: x = 3 - x
    od
}
