/* Found by differential testing against pan (generated model r1), checked
 * with `--progress`. All three processes end up blocked for good, P0 at its
 * progress label progress_0_0 inside an atomic sequence it cannot finish.
 * The np_ automaton sits on its accepting location and np_ is false while a
 * process is at a progress label, so it has no enabled edge there: no run
 * goes on from that state. pan -l and pan -l -f: no error. */
bit a, b;
byte d;
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { d != 2; skip; d = 0 }
	od
}
active proctype P1()
{
	do
	:: skip; progress_1_0: skip; if :: b != 1 -> b = 1 - b; b == 0; b == 0; d = (d + 1) % 3 :: else -> b == 1; d = 0 fi
	od
}
active proctype P2()
{
	do
	:: skip; progress_2_2: atomic { a = 1 - a; b == 0; d = (d + 1) % 3; a = 1 - a }
	od
}
