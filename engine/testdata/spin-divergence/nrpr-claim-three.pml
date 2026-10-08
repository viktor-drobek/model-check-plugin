/* Documented divergence from SPIN 6.5.2 (steps/fix-nrpr-confirmation.md):
 * pan counts the never claim in _nr_pr, this engine does not. Two processes
 * and a claim give pan _nr_pr == 3 in the initial state, so the claim ends at
 * once ("end state in claim reached", 3 states stored); the engine counts the
 * two processes only and the claim never fires (verified, 7 states).
 */
active proctype A() { skip }
active proctype B() { skip }
never { do :: (_nr_pr == 3) -> break :: else od }
