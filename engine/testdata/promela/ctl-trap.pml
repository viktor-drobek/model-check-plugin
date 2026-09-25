/* The same shape with a trap: from x = 2 there is no way back to x = 0, so
 * AG EF (x == 0) fails. The counterexample is the finite path to the trap;
 * why EF (x == 0) fails *there* is not a finite run, and the answer says so.
 */
byte x;

active proctype A()
{
end:	do
	:: x == 0 -> x = 1
	:: x == 1 -> x = 2
	:: x == 2 -> x = 2
	od
}
