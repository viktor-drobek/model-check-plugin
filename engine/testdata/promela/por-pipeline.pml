/* POR fixture (partial-order reduction of directed channels): a source, a
   filter and a sink over two buffered channels. Each channel has exactly one
   sender and one receiver, so a send and a receive on it do not depend on each
   other, and the processes need not be explored in every interleaving. The
   sink asserts that the K messages come out in order.
   mcd check --promela por-pipeline.pml -D K=3 --sweep --por */
#ifndef K
#define K 3
#endif
chan c = [2] of { byte };
chan d = [2] of { byte };

active proctype Source() {
  byte n = 0;
  do
  :: n < K -> c!n; n++
  :: else -> break
  od
}

active proctype Filter() {
  byte k = 0;
  byte m;
  do
  :: k < K -> c?m; d!m; k++
  :: else -> break
  od
}

active proctype Sink() {
  byte e = 0;
  byte v;
  do
  :: e < K -> d?v; assert(v == e); e++
  :: else -> break
  od
}
