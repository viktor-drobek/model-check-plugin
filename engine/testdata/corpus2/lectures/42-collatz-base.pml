/*
 * Source: books-md/lect01-lect09.md, lines 6481-6481 (lecture slide: the Collatz-like model used for the LTL examples)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The identical listing is repeated at markdown lines 6481, 6499, 6522 and 6550.
 */
int x = 100;

active proctype A() {
    do
    :: x%2 -> x = 3*x + 1
    od
}

active proctype B() {
    do
    :: !x%2 -> x = x/2
    od
}
