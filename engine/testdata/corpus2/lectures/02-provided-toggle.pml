/*
 * Source: books-md/lect01-lect09.md, lines 2333-2333 (lecture slide: explicit process synchronisation with provided clauses)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
bool toggle = true;
short cnt;

active proctype A() provided (toggle == true) {
L:      cnt++;
    printf("A: cnt=%d\n", cnt);
    toggle = false;
    goto L
}

active proctype B() provided (toggle == false) {
L:      cnt--;
    printf("B: cnt=%d\n", cnt);
    toggle = true;
    goto L
}
