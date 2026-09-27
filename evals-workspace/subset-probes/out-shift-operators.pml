/* probe: shift operators
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
byte n;
active proctype P() { n = (1 << 2) >> 1 }
