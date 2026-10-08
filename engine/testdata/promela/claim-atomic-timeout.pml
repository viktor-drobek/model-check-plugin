/* An acceptance cycle through an atomic sequence that begins with `timeout`.
 *
 * P loops forever on `atomic { timeout -> a = 1 - a }`, and Q has one step,
 * `timeout -> b = 1`. Both wait for the timeout, which is true when nothing
 * else can move, so the system alternates P's atomic step and the claim's
 * step, and Q is never the one that takes it. The never claim accepts while b
 * is 0: an acceptance cycle whose loop passes through the unstored
 * intermediate state of the atomic sequence (the statement after `timeout`).
 *
 * SPIN 6.5.2 (spin -a -o1 -o2 -o3; gcc -O2 -DNOREDUCE): pan -a reports the
 * acceptance cycle and 6 states stored with -c0.
 *
 * The published 0.2.0 stops on it with an index out of range, with and
 * without --sweep (the defect in the lasso of the nested search).
 */
bit a;
bit b;

active proctype P()
{
	do
	:: atomic { timeout -> a = 1 - a }
	od
}

active proctype Q()
{
	timeout -> b = 1
}

never {
accept_c:
	do
	:: (b == 0)
	od
}
