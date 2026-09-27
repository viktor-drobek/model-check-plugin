/* probe: ?: conditional expression
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
byte n;
active proctype P() { n = (1 > 0 -> 2 : 3) }
