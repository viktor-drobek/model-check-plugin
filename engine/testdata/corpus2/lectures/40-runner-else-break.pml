/*
 * Source: books-md/lect01-lect09.md, lines 5130-5130 (lecture slide: checking that a process has terminated, plain do..od)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype runner() {
    do
    :: ... ...
    :: else -> break
    od
}
