/*
 * Source: books-md/lect01-lect09.md, lines 2467-2467 (lecture slide: executability of run and of a bare expression statement)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «значение по умолчанию - 0 выполнимо, если получится создать процесс B будет выполнено, только если другой процесс изменит значение x»
 */
int x;

proctype A() {
    int y = 1;
    skip;
    run B();
    x = 2;
    (x > 2 && y == 1);
    printf('x %d, y %d\n', x, y)
}
