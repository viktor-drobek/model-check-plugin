/* probe: unless
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
byte n;
active proctype P() { { n = 1; n = 2 } unless { n > 0 -> n = 3 } }
