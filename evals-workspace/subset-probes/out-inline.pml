/* probe: inline name(args) { … }
 * claim: the reference's §3 says this is INSIDE the subset
 */
inline bump(v) { v = v + 1 }
byte n;
active proctype P() { bump(n) }
