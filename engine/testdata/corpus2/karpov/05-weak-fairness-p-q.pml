/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 4359-4359 (§6.5 example 6.7, the weak-fairness example: P loops until Q sets flag)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Printed line numbers removed; the book prints 4 twice and skips 8, and the listing is left exactly as printed.
 */
int n = 0;
bool flag = false;

active proctype P() {
 do
  :: flag -> break    /* завершается, как только flag равно true */
  :: else -> n = 1 - n
 od
}

active proctype Q() {
  flag = true        /* Q все время готов установить flag в true */
}
