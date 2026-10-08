/* An acceptance cycle that runs through an atomic sequence.
 *
 * The only process loops on an atomic pair of steps, so the loop of the
 * counterexample has to be drawn through the unstored intermediate state in
 * the middle of the atomic sequence (G1 rule: it is expanded but never
 * stored). The never claim accepts every run: the claim's location is an
 * accept location from the start, so the one cycle of the product is an
 * acceptance cycle (SPIN pan -a: acceptance cycle).
 */
bit y;

active proctype A()
{
	do
	:: atomic { y = 1; y = 0 }
	od
}

never {
accept:
	do
	:: true
	od
}
