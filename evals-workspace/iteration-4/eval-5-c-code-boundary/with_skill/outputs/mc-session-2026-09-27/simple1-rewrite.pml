/* Rewrite of "Promela - examples/CH17/simple1.pr" inside the engine's Promela subset.
   Declared change to the model: the embedded C (c_code/c_expr) is replaced by a
   Promela variable, Promela assignments and a Promela assert expression.
   Consequence: x becomes part of the state vector, which it is NOT in simple1.pr. */
int x;

active proctype simple()
{
	x = 2;
	if
	:: x = x + 2; assert(x == 4)
	:: x = x * 3; assert(x == 6)
	fi
}
