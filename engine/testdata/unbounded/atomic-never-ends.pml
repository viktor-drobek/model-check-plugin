/* An atomic block whose loop never ends: P keeps the exclusive control for ever,
 * so Q can never move. pan cannot answer either (its search is cut off by its
 * depth limit); the engine answers inconclusive (bounded): the depth budget for
 * the depth-first search, the bound of an atomic sequence (100 000 steps) for
 * the breadth-first one, mc_estimate and the CTL graph.
 *
 * The old frontend lowered the loop outside the block (it let go of the control
 * at the back edge), so this model ended quickly and the weak-fairness branch's
 * frontend change made the sequence real: the breadth-first search used to copy
 * the whole chain of moves at every step of the sequence and ran out of memory
 * (about 2 GB in two seconds, with no end in sight).
 */
bit x, y;

active proctype P()
{
	atomic {
		do
		:: x = 1 - x
		od
	}
}

active proctype Q()
{
	y = 1
}
