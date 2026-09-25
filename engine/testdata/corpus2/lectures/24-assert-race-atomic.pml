/*
 * Source: books-md/lect01-lect09.md, lines 4494-4494 (lecture slide: the same model guarded by two atomic sequences)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The slide is cut off: the closing brace of B() is missing in the book and is left missing here.
 */
byte state = 1;

active proctype A() {
    atomic((state == 1) -> state++);
    assert(state == 2)
}

active proctype B() {
    atomic((state == 1) -> state--);
    assert(state == 0)
