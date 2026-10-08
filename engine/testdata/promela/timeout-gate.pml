/* `timeout` is true only in a state where no statement of any process is
 * executable. At the initial state both timeouts are, and either process may
 * take one; after P1 has taken its timeout, P1's assignment is executable, so
 * P2's timeout is not, and the only move is the assignment. Simulation and
 * every search list the moves of a state the same way. */
bit a;
bit b;

active proctype P1()
{
	do
	:: timeout -> a = 1 - a
	od
}

active proctype P2()
{
	timeout -> b = 1
}
