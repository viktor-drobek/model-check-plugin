/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 3677-3683 (§5.6, the process Р1 of the mutual-exclusion example, atomic{})
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 3677, 3683.
 * The process name Р1 uses the Cyrillic letter Р and is kept as printed.
 */
proctype Р1() {
    bool NC1, C1 = false;
m1: NC1 = true;     /* процесс находится в некритической секции */
m2: NC1 = false;
    atomic {f==1; f=f-1};
m3: C1 = true;      /* процесс находится в критической секции */
m4: C1 = false;
    f=f+1;
m5: goto m1
}
