/* Documented divergence from SPIN 6.5.2 (steps/fix-nrpr-confirmation.md):
 * a model that reads _nr_pr and is checked with an LTL formula. pan checks
 * the formula with a never claim, and counts that claim in _nr_pr, so A can
 * never see (_nr_pr == 1) there; the engine's automaton is not counted and A
 * proceeds once B has ended.
 *
 *   <> (x == 1):  pan violated (acceptance cycle), engine verified
 *   [] (x == 0):  pan verified, engine violated
 *
 * Without a claim the two agree (pandiff, no -mode: no error, 5 states).
 */
byte x;
active proctype A() { (_nr_pr == 1); x = 1; do :: x = 1 od }
active proctype B() { skip }
