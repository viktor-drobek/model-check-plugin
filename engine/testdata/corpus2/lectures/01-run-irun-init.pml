/*
 * Source: books-md/lect01-lect09.md, lines 2287-2287 (lecture slide: the run operator, proctype parameters and _pid)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «>spin irun.pml it is me 1, 1 I created 1 and 2 it is me 2, 2 3 processes created >»
 */
proctype irun(byte x) {
    printf('it is me %d, %d\n',x,_pid)
}

init{
    pid a,b;
    a = run irun(1);
    b = run irun(2);
    printf('I created %d and %d', a, b)
}
