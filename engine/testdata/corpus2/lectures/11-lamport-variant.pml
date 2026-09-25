/*
 * Source: books-md/lect01-lect09.md, lines 2635-2635 (lecture slide: a variant of Lamport's bakery algorithm (1981))
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
byte turn[2];
byte mutex;

active [2] proctype P() {
    bit i = _pid;
L:
    turn[i] = 1;
    turn[i] = turn[i+1];
    (turn[1-i] == 0) || (turn[i] < turn[1-i]) ->
    mutex++;
    assert(mutex == 1);
    mutex--;
    turn[i] = 0;
    goto L;
}
