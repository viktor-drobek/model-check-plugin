/* Two-process starvation: A spins forever (flipping its own bit), B
 * wants to set `done`. Without fairness the run in which only A moves
 * is a legal execution, so <>done is violated; under weak fairness B,
 * which is continuously enabled, must eventually move, so <>done holds
 * (SPIN: pan -a finds an acceptance cycle, pan -a -f does not). */
bit done;

active proctype A()
{	bit t;
	do
	:: t = 1 - t
	od
}

active proctype B()
{
	done = 1
}
