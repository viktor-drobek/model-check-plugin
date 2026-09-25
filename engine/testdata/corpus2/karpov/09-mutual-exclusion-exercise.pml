/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 4825-4825 (§7.6 exercise 7.2, a mutual-exclusion algorithm to be checked by the reader)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bool  turn, flag[2];
byte  ncrit;

active proctype[2] user() {
    assert ( _pid == 0 ||  _pid ==1 );
start:
    flag[ _pid ] = 1;
    turn = _pid;
    (flag [1-_pid] == 0 || turn == 1 - _pid);
    ncrit++;
    assert (ncrit == 1);
    /* critical section */
    ncrit --;
    flag [_pid ] = 0;
    goto start;
}
