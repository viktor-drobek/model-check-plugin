/*
 * Source: books-md/lect01-lect09.md, lines 4660-4660 (lecture slide: the same model with two variables and weak fairness)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte x = 2, y = 2;

active proctype A() {
    do
    :: x = 3 - x;
    :: y = 3 - y
    od
}

active proctype B() {
    do
    :: x = 3 - x;
    :: y = 3 - y
    od
}
