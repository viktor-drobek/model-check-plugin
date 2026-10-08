/* D1: a system that stops (deadlock) with p false; property <>[] p ("p eventually holds for good").
   Claim = spin -f '!(<>[] p)'. Stuttering run: p false forever -> violates <>[]p. */
bit a;
#define p (a == 1)
active proctype P() { a == 1 }
never  {    /* !(<>[] p) */
T0_init:
	do
	:: (! ((p))) -> goto accept_S9
	:: (1) -> goto T0_init
	od;
accept_S9:
	do
	:: (1) -> goto T0_init
	od;
}
