/*
 * Source: books-md/lect01-lect09.md, lines 4978-4978 (lecture slide: process A of mutex2 used to illustrate traces)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bit x,y;
byte mutex;

active proctype A() {
    x = 1;
    (y == 0) ->
    mutex++;
    printf('%d\n', _pid);
    mutex--;
    x = 0
}
