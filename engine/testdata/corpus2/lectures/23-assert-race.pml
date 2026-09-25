/*
 * Source: books-md/lect01-lect09.md, lines 4472-4472 (lecture slide: assertions, simple.pml before the race is fixed)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte state = 1;

active proctype A() {
    assert(state == 2)
}

active proctype B() {
    assert(state == 0)
}
