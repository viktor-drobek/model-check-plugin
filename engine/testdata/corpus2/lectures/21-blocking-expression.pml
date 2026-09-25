/*
 * Source: books-md/lect01-lect09.md, lines 4268-4268 (lecture slide: the same wait written as a blocking expression)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bit b;

active proctype {
    (b == 1) -> A
}
