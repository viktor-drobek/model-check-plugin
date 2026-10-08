/* A process that is blocked for good (B) in front of a process that lives on
 * `timeout` moves (P), no never claim: the accept label of P is the property.
 * Under weak fairness the copy of B is left by a null step in every state, and
 * the timeout moves of the system must be enumerated in that same frame: the
 * product has 16 states (testdata/weakdecision/copies_tc3.py derives the number
 * from the construction alone). The engine counted the null step among the
 * moves of the frame, so the timeout phase never started there: the moves were
 * lost and a stutter step was offered instead. */
bit a;

active proctype B()
{
	0 == 1
}

active proctype P()
{
	do
	:: accept_p: timeout -> a = 1 - a
	od
}
