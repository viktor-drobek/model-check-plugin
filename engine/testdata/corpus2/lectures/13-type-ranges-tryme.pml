/*
 * Source: books-md/lect01-lect09.md, lines 2901-2901 (lecture slide: expression evaluation and truncation on assignment)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
mtype = {foo, bar};

active proctype tryme() {
    byte x;
    short y = 1024;
    chan a,b;
    mtype p;
    a = a + b;
    x = 257;
    x = y;
    p = y/8
}
