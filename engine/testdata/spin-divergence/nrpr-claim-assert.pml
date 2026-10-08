/* Documented divergence from SPIN 6.5.2 (steps/fix-nrpr-confirmation.md):
 * under a never claim pan's _nr_pr is one larger than this engine's, so
 * (_nr_pr == 1) in A is true here once B has ended and never in pan, which
 * therefore never reaches the failing assert.
 * pan -a -c0: no error, 3 states stored. engine: assertion violated, 6 states.
 */
active proctype A() { (_nr_pr == 1); assert(false) }
active proctype B() { skip }
never { do :: skip od }
