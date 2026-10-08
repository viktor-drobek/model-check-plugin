/* An acceptance cycle through an atomic pair that starves a second process.
 *
 * P loops forever on `atomic { x = 1 - x; x = 1 - x }`; Q has one step, y = 1,
 * and is never scheduled on the run that stays in P. The never claim accepts
 * while y is 0, so that run is an acceptance cycle, and its loop passes
 * through the unstored intermediate state in the middle of the atomic pair.
 *
 * SPIN 6.5.2 (spin -a -o1 -o2 -o3; gcc -O2 -DNOREDUCE): pan -a reports the
 * acceptance cycle and 2 states stored (-c0 too); pan -a -f (weak fairness)
 * reports none, since Q is enabled throughout the loop and must move.
 *
 * A second case of the defect in the lasso of the nested search, found on a
 * model that is not a pair of statements of one process: the published 0.2.0
 * stops on it with an index out of range, with and without --sweep.
 */
bit x, y;

active proctype P()
{
	do
	:: atomic { x = 1 - x; x = 1 - x }
	od
}

active proctype Q()
{
	y = 1
}

never {
accept_c:
	do
	:: (y == 0)
	od
}
