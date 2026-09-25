/*
 * Rewrite of the corpus model  Promela - examples/CH17/simple1.pr
 *   source sha256 d7a34d649c9cc83ecb852dd2ca584c1ce3b977fceb5a2dbb5d09db542b2d4b70
 * The original is NOT copied here. It uses c_code / c_expr, which are outside the
 * engine's subset (embedded C, NFR-004). This file models the effect of the C
 * fragments as ordinary Promela assignments and expressions, as
 * references/promela-subset.md §3 prescribes.
 *
 * Declared changes w.r.t. the original (see report §3):
 *   1. `c_code { int x; }`      -> `int x;`   (x becomes a tracked model variable)
 *   2. `c_code { x = 2; }`      -> `x = 2`
 *   3. `c_code { x = x+2; }`    -> `x = x + 2`
 *   4. `c_code { x = x*3; }`    -> `x = x * 3`
 *   5. `assert(c_expr { x==4 })`-> `assert(x == 4)`
 *   6. `assert(c_expr { x==6 })`-> `assert(x == 6)`
 */

int x;

active proctype simple()
{
	x = 2;
	if
	:: x = x + 2; assert(x == 4)
	:: x = x * 3; assert(x == 6)
	fi
}
