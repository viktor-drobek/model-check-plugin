/* probe: unless
 * claim: the reference's §1 says this is INSIDE the subset
 */
byte n;
active proctype P() { { n = 1; n = 2 } unless { n > 0 -> n = 3 } }
