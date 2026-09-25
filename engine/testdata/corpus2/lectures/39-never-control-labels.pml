/*
 * Source: books-md/lect01-lect09.md, lines 5124-5124 (lecture slide: a never claim referring to control labels of active processes)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «Используем метки управления вместо счётчика процессов»
 * HTML entities decoded: &amp; -> &, &gt; -> >, and the markdown escapes \_pid -> _pid
 */
never {
    do
    :: user[1]@crit && user[2]@crit -> break
    :: else
    od
}

mtype = {p, v};
chan sem = [0] of { mtype };

active proctype semaphore() {
    do
    sem!p ;
    sem?v
    od
}

active [2] proctype user() {
    assert(_pid == 1 || _pid == 2);
    do
    :: sem?p ->
crit:   /*критическая секция*/
        sem!v
    od
}
