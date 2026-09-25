/* Rewrite of "Promela - examples/CH17/simple1.pr" inside the engine's Promela subset.
   The embedded C (c_code / c_expr) is replaced by Promela assignments and guards
   with the same arithmetic effect on x. Assumption (default, not confirmed by the
   user): the C blocks have no effect other than the shown update of x, and each
   c_code block is one atomic step (as SPIN executes it). Note: in SPIN a c_code
   variable is NOT part of the state vector unless c_track/c_state is used; here x
   is an ordinary Promela int, so it IS part of the state. */
int x;

active proctype simple()
{
   x = 2;
   if
   :: x = x+2; assert(x == 4)
   :: x = x*3; assert(x == 6)
   fi
}
