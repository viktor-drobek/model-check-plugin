/*
 * Source: books-md/lect01-lect09.md, lines 5010-5010 (lecture slide: the wrong sequential variant of the same property)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype invariant() {
    p;
    do
    ::assert(!q);
    od
}
