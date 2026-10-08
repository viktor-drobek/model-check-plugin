/* Two acceptance cycles, the first without an atomic sequence, the second
 * through one.
 *
 * The search finds the cycle on x first (the first alternative of the loop)
 * and decides the property. A search that goes on after the verdict (mcd
 * check --sweep, pan -c0) finds the cycle on y next: it passes through the
 * unstored intermediate state of the atomic pair. Without --sweep the run
 * stops at the first cycle and never sees the second.
 *
 * The never claim accepts every run, so every cycle of the product is an
 * acceptance cycle (SPIN pan -a: acceptance cycle).
 */
bit x, y;

active proctype A()
{
	do
	:: x = 1 - x
	:: atomic { y = 1; y = 0 }
	od
}

never {
accept:
	do
	:: true
	od
}
