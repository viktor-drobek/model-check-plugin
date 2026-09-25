/*
 * Source: books-md/lect01-lect09.md, lines 5004-5004 (lecture slide: trying to express 'p is never followed by q' with one assert)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype invariant() {
    assert(!p || !q);
}
