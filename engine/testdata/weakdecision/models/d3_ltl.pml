/* D3 natural form: LTL <> (b==1) ("the timeout-only process Q eventually fires"). Claim = spin -f '!(<> q)'. */
bit b;
#define q (b == 1)
active proctype P() { do :: timeout od }
active proctype Q() { timeout -> b = 1 }
never  {    /* !(<> q) */
accept_init:
T0_init:
	do
	:: (! ((q))) -> goto T0_init
	od;
}
