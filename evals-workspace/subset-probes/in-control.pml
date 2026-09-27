/* probe: if/fi, do/od, ::, ->, else, break, goto, labels
 * claim: the reference's §1 Control says this is INSIDE the subset
 */
active proctype P() { byte n;
again: if :: n == 0 -> n = 1 :: else -> goto again fi;
  do :: n > 0 -> n--; break :: else -> break od }
