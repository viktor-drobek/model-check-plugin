/* Documented divergence from SPIN 6.5.2 (steps/fix-nrpr-confirmation.md):
 * pan counts the never claim in _nr_pr, this engine does not. With two
 * processes that end at once, _nr_pr == 0 is reached here (the claim sees the
 * model's own count, 0) and is never reached in pan, whose count is 1 or more
 * for as long as the claim exists.
 * pan -a -c0: no error, 7 states stored. engine: never violated, 8 states.
 */
active proctype A() { skip }
active proctype B() { skip }
never { do :: (_nr_pr == 0) -> break :: else od }
