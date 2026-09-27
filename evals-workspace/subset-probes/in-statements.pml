/* probe: assignment, guard, assert, skip, true, false, printf
 * claim: the reference's §1 Statements says this is INSIDE the subset
 */
byte n;
active proctype P() { n = 1; n > 0; assert(n == 1); skip; true; printf("n=%d\n", n) }
