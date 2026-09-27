/* probe: bit, bool, byte, short, int, pid
 * claim: the reference's §1 Types says this is INSIDE the subset
 */
bit a; bool b; byte c; short d; int e; pid f;
active proctype P() { a = 1; b = true; c = 2; d = 3; e = 4; f = 0 }
