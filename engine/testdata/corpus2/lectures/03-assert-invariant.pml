/*
 * Source: books-md/lect01-lect09.md, lines 2457-2457 (lecture slide: assert as a safety (state) property monitor)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
int n;

active proctype invariant() {
    assert(n <= 3)
}
