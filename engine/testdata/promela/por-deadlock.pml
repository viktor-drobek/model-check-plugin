/* POR fixture: P and Q wait for each other's flag (a deadlock), while three
   workers count privately. The workers' interleavings are what a reduction
   removes; the deadlock of P and Q must survive it. */
bool f1 = false;
bool f2 = false;
byte w[5];

active proctype P() { f2 -> f1 = true }
active proctype Q() { f1 -> f2 = true }

active [3] proctype W() {
  byte i = 0;
  do
  :: i < 3 -> i++; w[_pid] = i
  :: else -> break
  od
}
