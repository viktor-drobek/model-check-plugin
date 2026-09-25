/*
 * Source: books-md/lect01-lect09.md, lines 4016-4016 (lecture slide: abstraction example, do/break/else plus a non-deterministic init)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The identical listing is repeated at markdown lines 4016 and 4034.
 */
proctype f(int x){
    do
    :: x < 3 -> printf('Case 1\n'); x = x + 1
    :: x >= 10 -> break;
    :: else -> printf('Case 2\n'); x = x + 1
    od
}

init {
    int x;
    do
    :: x++;
    :: x--;
    :: break;
    od;
    run f(x)
}
