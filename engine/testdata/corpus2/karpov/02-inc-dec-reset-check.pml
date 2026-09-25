/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 3737-3743 (§5.6 example 5.5, a monitor process Check() verifying an invariant of Inc/Dec/Reset)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 3737, 3743.
 * Printed line numbers removed; the book skips number 5 and the listing is left exactly as printed.
 */
x=0;

proctype Inc() {
  do :: true -> if :: x<10 -> x = x+1 fi od }

proctype Dec() {
  do :: true -> if :: x>0 -> x = x-1 fi od }

proctype Reset() {
  do :: true -> if :: x==10 -> x = 0 fi od }

proctype Check() {
  assert ( x>= 0 && x<=10 ) }

init {
  atomic{ run Inc();run Dec();run Reset();run Check();
}
