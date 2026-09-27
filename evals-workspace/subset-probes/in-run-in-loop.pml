/* probe: run inside a loop (G5)
 * claim: the reference's §1 Processes says this is INSIDE the subset
 */
byte n;
proctype W() { skip }
init { do :: n < 2 -> n++; run W() :: else -> break od }
